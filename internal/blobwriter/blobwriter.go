// Package blobwriter provides a transactional writer for content-addressable
// blob files on a local filesystem.
//
// A BlobWriter stages the incoming bytes in an unconfirmed temporary file; the
// blob becomes a reusable candidate only when Commit() completes all of the
// following steps:
//
//  1. The declared size (if known) matches the bytes actually present
//     (rejecting both short writes and trailing content).
//  2. The declared canonical digest (if known) matches the actual file
//     contents.
//  3. The temporary file is fsync'ed and closed.
//  4. The blob is published at its final, content-addressable path using an
//     atomic hard link (never overwriting an already-confirmed blob).
//
// Any failure before step 4 removes only this writer's temporary file; final
// blob paths are never deleted by a failing writer. Different writers (e.g.
// independent network retries) use independent temporary files.
//
// The writer is deliberately a low-level byte sink: compression, encryption
// and digest verification of the incoming stream happen in the layers above
// (as they do in the copy pipeline), chunked uploads keep using their own
// code path (or can be assembled using WriteAt), and Seek is supported with
// the final digest always verified against the bytes on disk.
package blobwriter

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/opencontainers/go-digest"
	"github.com/sirupsen/logrus"
)

const (
	// stagingPrefix marks temporary files which have never been confirmed by
	// Commit(). Any file with this prefix is safe to remove by
	// CleanupStaging or by the owning writer.
	stagingPrefix = ".blobwriter-ingest-"
	// lockFileName is the advisory lock coordinating active writers with
	// CleanupStaging.
	lockFileName = ".blobwriter.lock"
)

var (
	// ErrSizeMismatch indicates that the staged blob size does not match the
	// declared size (a short write or unexpected trailing bytes).
	ErrSizeMismatch = errors.New("blob size mismatch")
	// ErrDigestMismatch indicates that the staged blob contents do not match
	// the declared canonical digest.
	ErrDigestMismatch = errors.New("blob digest mismatch")
	// ErrTrailingData indicates that more bytes than the declared size were
	// offered to the writer.
	ErrTrailingData = errors.New("blob contains more bytes than declared")
)

// errBlobWriterDone is returned when a writer is used after Commit or Close.
var errBlobWriterDone = errors.New("blob writer already committed or closed")

// Options configures a BlobWriter.
type Options struct {
	// ExpectedDigest, if non-empty, names the blob expected to be written.
	// A digest using digest.Canonical is verified against the actual staged
	// contents. Digests using other algorithms are accepted like an unknown
	// digest (the returned blob identity is the canonical digest computed by
	// the writer).
	ExpectedDigest digest.Digest
	// ExpectedSize is the exact number of bytes the blob must contain.
	// Set to -1 if the size is not known.
	ExpectedSize int64
	// FinalPath returns the final content-addressable path for the canonical
	// digest computed from the written contents. The path's parent directory
	// must exist and must reside on the same filesystem as StagingDir (hard
	// links can not span filesystems).
	FinalPath func(computed digest.Digest) (string, error)
}

// BlobWriter writes a single blob as one recoverable commit.
type BlobWriter struct {
	stagingDir string
	opts       Options

	file     *os.File // Unconfirmed staging file, or nil once closed
	tempPath string   // Name of the unconfirmed staging file

	lock *os.File // Holds a shared advisory lock for the writer lifetime

	// Streaming canonical digest of the bytes passed to Write. Authoritative
	// for the usual linear write path only.
	digester digest.Digester
	offset   int64 // Current staging file offset as tracked by Write/Seek
	// linear is true while all writes happened at strictly contiguous offsets
	// starting at 0 (no Seek/WriteAt). When false, Commit hashes the file on
	// disk instead of trusting the streaming digest.
	linear bool

	done bool // Commit or Close has finalized the writer

	// syncFile persists the staging file contents; it is a field to allow
	// fsync failures to be exercised in tests.
	syncFile func(f *os.File) error
}

// New creates a BlobWriter staging a new temporary file in stagingDir.
// stagingDir must exist and must reside on the same filesystem as the paths
// returned by Options.FinalPath.
// The returned writer holds a shared advisory lock until Close or Commit.
func New(stagingDir string, opts *Options) (*BlobWriter, error) {
	if opts == nil {
		return nil, errors.New("creating blob writer: no options specified")
	}
	if opts.FinalPath == nil {
		return nil, errors.New("creating blob writer: no FinalPath function specified")
	}
	if opts.ExpectedSize < -1 {
		return nil, fmt.Errorf("creating blob writer: invalid expected size %d", opts.ExpectedSize)
	}
	if opts.ExpectedDigest != "" {
		if err := opts.ExpectedDigest.Validate(); err != nil {
			return nil, fmt.Errorf("creating blob writer: invalid expected digest %q: %w", opts.ExpectedDigest, err)
		}
	}
	fi, err := os.Stat(stagingDir)
	if err != nil {
		return nil, fmt.Errorf("opening blob staging directory %q: %w", stagingDir, err)
	}
	if !fi.IsDir() {
		return nil, fmt.Errorf("blob staging path %q is not a directory", stagingDir)
	}

	lock, err := os.OpenFile(filepath.Join(stagingDir, lockFileName), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, fmt.Errorf("opening staging lock in %q: %w", stagingDir, err)
	}
	if err := sharedLock(lock); err != nil {
		_ = lock.Close()
		return nil, fmt.Errorf("acquiring staging lock in %q: %w", stagingDir, err)
	}

	file, err := os.CreateTemp(stagingDir, stagingPrefix+"*")
	if err != nil {
		_ = unlock(lock)
		_ = lock.Close()
		return nil, fmt.Errorf("creating staging file in %q: %w", stagingDir, err)
	}

	return &BlobWriter{
		stagingDir: stagingDir,
		opts:       *opts,
		file:       file,
		tempPath:   file.Name(),
		lock:       lock,
		digester:   digest.Canonical.Digester(),
		linear:     true,
		syncFile:   (*os.File).Sync,
	}, nil
}

// Write appends bytes to the unconfirmed staging file.
// It fails (without writing) if accepting the bytes would exceed the declared
// size, so that trailing content can never become part of the blob.
func (w *BlobWriter) Write(p []byte) (int, error) {
	if w.done {
		return 0, errBlobWriterDone
	}
	if w.opts.ExpectedSize >= 0 && w.offset+int64(len(p)) > w.opts.ExpectedSize {
		return 0, fmt.Errorf("%w: writing %d bytes at offset %d exceeds declared size %d",
			ErrTrailingData, len(p), w.offset, w.opts.ExpectedSize)
	}
	n, err := w.file.Write(p)
	w.offset += int64(n)
	if n > 0 {
		if _, hashErr := w.digester.Hash().Write(p[:n]); hashErr != nil { // sha256.Hash.Write never fails
			return n, hashErr
		}
	}
	return n, err
}

// WriteAt writes bytes at the given offset, allowing non-sequential assembly
// of blob chunks. It marks the write path as non-linear, so Commit verifies
// the digest of the complete file on disk.
func (w *BlobWriter) WriteAt(p []byte, off int64) (int, error) {
	if w.done {
		return 0, errBlobWriterDone
	}
	if off < 0 {
		return 0, fmt.Errorf("writing blob at negative offset %d", off)
	}
	if w.opts.ExpectedSize >= 0 && off+int64(len(p)) > w.opts.ExpectedSize {
		return 0, fmt.Errorf("%w: writing %d bytes at offset %d exceeds declared size %d",
			ErrTrailingData, len(p), off, w.opts.ExpectedSize)
	}
	n, err := w.file.WriteAt(p, off)
	w.linear = false
	return n, err
}

// Seek sets the offset for the next Write. Seek marks the write path as
// non-linear, so Commit always verifies the digest of the actual file
// contents after any seek.
func (w *BlobWriter) Seek(offset int64, whence int) (int64, error) {
	if w.done {
		return 0, errBlobWriterDone
	}
	pos, err := w.file.Seek(offset, whence)
	if err != nil {
		return 0, err
	}
	w.offset = pos
	w.linear = false
	return pos, nil
}

// Commit validates size and digest, fsyncs the staging file and publishes the
// blob at its final path. On success the returned digest identifies the blob
// and size is its length.
// On failure the unconfirmed staging file is discarded and the error is
// returned; a final, already-confirmed blob is never modified or removed.
func (w *BlobWriter) Commit() (digest.Digest, int64, error) {
	if w.done {
		return "", -1, errBlobWriterDone
	}

	// Determine the digest of the actual staged contents.
	var computed digest.Digest
	if w.linear {
		computed = w.digester.Digest()
	} else {
		rehashed, err := digestFile(w.file)
		if err != nil {
			return "", -1, w.abort(err)
		}
		computed = rehashed
	}

	fi, err := w.file.Stat()
	if err != nil {
		return "", -1, w.abort(fmt.Errorf("stat'ing staged blob: %w", err))
	}
	finalSize := fi.Size()

	if w.opts.ExpectedSize >= 0 && finalSize != w.opts.ExpectedSize {
		return "", -1, w.abort(fmt.Errorf("%w: expected %d bytes, got %d",
			ErrSizeMismatch, w.opts.ExpectedSize, finalSize))
	}
	if w.opts.ExpectedDigest != "" &&
		w.opts.ExpectedDigest.Algorithm() == digest.Canonical &&
		w.opts.ExpectedDigest != computed {
		return "", -1, w.abort(fmt.Errorf("%w: expected %s, got %s",
			ErrDigestMismatch, w.opts.ExpectedDigest, computed))
	}

	finalPath, err := w.opts.FinalPath(computed)
	if err != nil {
		return "", -1, w.abort(err)
	}

	// Durability gate: the contents must be on stable storage before the blob
	// can become visible under any digest.
	if err := w.syncFile(w.file); err != nil {
		return "", -1, w.abort(fmt.Errorf("syncing staged blob %s: %w", computed, err))
	}

	// On POSIX systems, the staging file was created with mode 0600, so make
	// it readable; on Windows chmod is unsupported and unnecessary.
	if runtime.GOOS != "windows" {
		if err := w.file.Chmod(0644); err != nil {
			return "", -1, w.abort(fmt.Errorf("making staged blob %s readable: %w", computed, err))
		}
	}

	if err := w.file.Close(); err != nil {
		w.file = nil // removeStaging must not double-close
		return "", -1, w.abort(fmt.Errorf("closing staged blob %s: %w", computed, err))
	}
	w.file = nil

	if err := w.publish(finalPath, computed, finalSize); err != nil {
		return "", -1, w.abort(err)
	}

	w.finish()
	return computed, finalSize, nil
}

// publish atomically installs the staged blob at finalPath using a hard link.
// If finalPath already holds a confirmed blob, it is verified and left
// untouched; it is never replaced or deleted.
func (w *BlobWriter) publish(finalPath string, computed digest.Digest, finalSize int64) error {
	if err := os.Link(w.tempPath, finalPath); err == nil {
		w.discardTemp()
		// Best-effort persistence of the new directory entry. A failure here
		// can at worst make the publish invisible after a crash (the caller
		// retries); it can never leave a wrong candidate, so it is advisory.
		if err := syncDirectory(filepath.Dir(finalPath)); err != nil {
			logrus.Debugf("syncing directory %q after publishing blob %s: %v", filepath.Dir(finalPath), computed, err)
		}
		return nil
	} else if !errors.Is(err, fs.ErrExist) {
		return fmt.Errorf("publishing blob %s at %q: %w", computed, finalPath, err)
	}

	// A blob already exists at the final path. Verify that it is the same
	// confirmed blob; never overwrite or delete it.
	fi, err := os.Stat(finalPath)
	if err != nil {
		return fmt.Errorf("checking existing blob at %q: %w", finalPath, err)
	}
	if !fi.Mode().IsRegular() || fi.Size() != finalSize {
		return fmt.Errorf("existing blob at %q is not the expected blob (size %d, want %d)",
			finalPath, fi.Size(), finalSize)
	}
	f, err := os.Open(finalPath)
	if err != nil {
		return fmt.Errorf("opening existing blob at %q: %w", finalPath, err)
	}
	existingDigest, err := digest.FromReader(f)
	f.Close()
	if err != nil {
		return fmt.Errorf("reading existing blob at %q: %w", finalPath, err)
	}
	if existingDigest != computed {
		return fmt.Errorf("%w: existing blob at %q has digest %s, expected %s",
			ErrDigestMismatch, finalPath, existingDigest, computed)
	}
	// The confirmed blob already in place is identical; our staging file is
	// redundant.
	w.discardTemp()
	return nil
}

// digestFile returns the canonical digest of f's contents by reading it from
// the beginning; the file offset is restored afterwards.
func digestFile(f *os.File) (digest.Digest, error) {
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return "", fmt.Errorf("rewinding staged blob for digest verification: %w", err)
	}
	computed, err := digest.FromReader(f)
	if err != nil {
		return "", fmt.Errorf("reading staged blob for digest verification: %w", err)
	}
	return computed, nil
}

// discardTemp removes this writer's unconfirmed staging file. It is called
// only after the blob contents are confirmed at the final path; errors are
// left for the next CleanupStaging.
func (w *BlobWriter) discardTemp() {
	if w.file != nil {
		_ = w.file.Close()
		w.file = nil
	}
	if w.tempPath == "" {
		return
	}
	if err := os.Remove(w.tempPath); err != nil && !os.IsNotExist(err) {
		logrus.Debugf("removing staging file %q after publishing: %v", w.tempPath, err)
	}
	w.tempPath = ""
}

// abort discards this writer's unconfirmed candidate because of cause. Only
// the writer's own staging file may be removed.
func (w *BlobWriter) abort(cause error) error {
	w.removeStaging()
	w.releaseLock()
	w.done = true
	return cause
}

// finish releases writer resources after a successful commit.
func (w *BlobWriter) finish() {
	w.releaseLock()
	w.done = true
}

// removeStaging closes and removes the unconfirmed staging file. Final blob
// paths are never touched.
func (w *BlobWriter) removeStaging() {
	if w.file != nil {
		_ = w.file.Close()
		w.file = nil
	}
	if w.tempPath == "" {
		return
	}
	if err := os.Remove(w.tempPath); err != nil && !os.IsNotExist(err) {
		logrus.Debugf("removing unconfirmed staging file %q: %v", w.tempPath, err)
	}
	w.tempPath = ""
}

// releaseLock releases the shared advisory lock.
func (w *BlobWriter) releaseLock() {
	if w.lock == nil {
		return
	}
	_ = unlock(w.lock)
	_ = w.lock.Close()
	w.lock = nil
}

// Close abandons the blob when it has not been committed, removing the
// unconfirmed staging file. It is safe to call after a successful Commit.
func (w *BlobWriter) Close() error {
	if w.done {
		return nil
	}
	w.removeStaging()
	w.releaseLock()
	w.done = true
	return nil
}

// CleanupStaging removes all unconfirmed temporary files left in stagingDir by
// writers that never reached Commit() (e.g. after a process crash). Files not
// matching the staging prefix are left untouched.
// An exclusive advisory lock ensures files still being written by active
// writers (within or outside this process) are never removed.
func CleanupStaging(stagingDir string) error {
	lock, err := os.OpenFile(filepath.Join(stagingDir, lockFileName), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return fmt.Errorf("opening staging lock in %q: %w", stagingDir, err)
	}
	defer lock.Close()
	if err := exclusiveLock(lock); err != nil {
		return fmt.Errorf("acquiring staging lock in %q: %w", stagingDir, err)
	}
	defer unlock(lock) //nolint:errcheck

	entries, err := os.ReadDir(stagingDir)
	if err != nil {
		return fmt.Errorf("reading staging directory %q: %w", stagingDir, err)
	}
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), stagingPrefix) {
			continue
		}
		if !entry.Type().IsRegular() {
			continue
		}
		path := filepath.Join(stagingDir, entry.Name())
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("removing unconfirmed staging file %q: %w", path, err)
		}
	}
	return nil
}
