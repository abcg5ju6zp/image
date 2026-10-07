package blobwriter

import (
	"bytes"
	_ "crypto/sha512" // Register sha512 with go-digest
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/opencontainers/go-digest"
)

// finalPathInDir resolves the final path of dg inside dir.
func finalPathInDir(dir string) func(digest.Digest) (string, error) {
	return func(dg digest.Digest) (string, error) {
		return filepath.Join(dir, dg.Encoded()), nil
	}
}

func newBlobWriter(t *testing.T, dir string, expectedDigest digest.Digest, expectedSize int64) *BlobWriter {
	t.Helper()
	w, err := New(dir, &Options{
		ExpectedDigest: expectedDigest,
		ExpectedSize:   expectedSize,
		FinalPath:      finalPathInDir(dir),
	})
	if err != nil {
		t.Fatalf("New(%q): %v", dir, err)
	}
	return w
}

func assertNoStagingFiles(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir(%q): %v", dir, err)
	}
	for _, e := range entries {
		if bytes.HasPrefix([]byte(e.Name()), []byte(stagingPrefix)) {
			t.Errorf("unconfirmed staging file %q left in %q", e.Name(), dir)
		}
	}
}

func TestCommitSuccess(t *testing.T) {
	dir := t.TempDir()
	content := []byte("hello world, this is a complete blob")
	expectedDigest := digest.FromBytes(content)

	w := newBlobWriter(t, dir, expectedDigest, int64(len(content)))
	if _, err := io.Copy(w, bytes.NewReader(content)); err != nil {
		t.Fatalf("writing blob: %v", err)
	}
	gotDigest, gotSize, err := w.Commit()
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if gotDigest != expectedDigest {
		t.Errorf("digest = %q, want %q", gotDigest, expectedDigest)
	}
	if gotSize != int64(len(content)) {
		t.Errorf("size = %d, want %d", gotSize, len(content))
	}
	if err := w.Close(); err != nil {
		t.Errorf("Close after Commit: %v", err)
	}
	persisted, err := os.ReadFile(filepath.Join(dir, expectedDigest.Encoded()))
	if err != nil {
		t.Fatalf("reading published blob: %v", err)
	}
	if !bytes.Equal(persisted, content) {
		t.Errorf("published blob contents = %q, want %q", persisted, content)
	}
	assertNoStagingFiles(t, dir)
}

func TestCommitUnknownSizeAndDigest(t *testing.T) {
	dir := t.TempDir()
	content := []byte("size and digest are unknown while writing")
	expectedDigest := digest.FromBytes(content)

	w := newBlobWriter(t, dir, "", -1)
	if _, err := w.Write(content); err != nil {
		t.Fatalf("Write: %v", err)
	}
	gotDigest, gotSize, err := w.Commit()
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if gotDigest != expectedDigest {
		t.Errorf("digest = %q, want %q", gotDigest, expectedDigest)
	}
	if gotSize != int64(len(content)) {
		t.Errorf("size = %d, want %d", gotSize, len(content))
	}
}

func TestCommitEmptyBlob(t *testing.T) {
	dir := t.TempDir()
	expectedDigest := digest.FromBytes(nil) // sha256 of empty input

	w := newBlobWriter(t, dir, expectedDigest, 0)
	gotDigest, gotSize, err := w.Commit()
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if gotDigest != expectedDigest || gotSize != 0 {
		t.Errorf("got (%q, %d), want (%q, 0)", gotDigest, gotSize, expectedDigest)
	}
}

func TestCommitShortWrite(t *testing.T) {
	dir := t.TempDir()
	content := []byte("only three bytes") // 15 bytes

	w := newBlobWriter(t, dir, digest.FromBytes(content), int64(len(content))+10)
	if _, err := w.Write(content); err != nil {
		t.Fatalf("Write: %v", err)
	}
	_, _, err := w.Commit()
	if !errors.Is(err, ErrSizeMismatch) {
		t.Fatalf("Commit error = %v, want ErrSizeMismatch", err)
	}
	if err := w.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
	assertNoStagingFiles(t, dir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != lockFileName { // only the lock file is expected
		names := make([]string, len(entries))
		for i, e := range entries {
			names[i] = e.Name()
		}
		t.Errorf("directory contents = %v, want only %q", names, lockFileName)
	}
}

func TestWriteTrailingContent(t *testing.T) {
	dir := t.TempDir()
	content := []byte("these are definitely more than five bytes")

	w := newBlobWriter(t, dir, "", 5)
	first := []byte("12345")
	if _, err := w.Write(first); err != nil {
		t.Fatalf("Write of declared size: %v", err)
	}
	if _, err := w.Write([]byte("6")); !errors.Is(err, ErrTrailingData) {
		t.Fatalf("Write beyond declared size error = %v, want ErrTrailingData", err)
	}
	// Large trailing write is rejected wholesale.
	if _, err := w.Write(content); !errors.Is(err, ErrTrailingData) {
		t.Fatalf("trailing Write error = %v, want ErrTrailingData", err)
	}
	// Nothing may have been committed...
	if _, _, err := w.Commit(); err != nil {
		t.Fatalf("Commit with exactly the declared bytes: %v", err)
	}
	assertNoStagingFiles(t, dir)
}

func TestCommitDigestMismatch(t *testing.T) {
	dir := t.TempDir()
	content := []byte("the real contents")

	w := newBlobWriter(t, dir, digest.FromBytes([]byte("something else")), int64(len(content)))
	if _, err := w.Write(content); err != nil {
		t.Fatalf("Write: %v", err)
	}
	_, _, err := w.Commit()
	if !errors.Is(err, ErrDigestMismatch) {
		t.Fatalf("Commit error = %v, want ErrDigestMismatch", err)
	}
	assertNoStagingFiles(t, dir)
}

func TestCloseAbandonsUnconfirmedBlob(t *testing.T) {
	// Simulates a disconnected/aborted stream: the writer is closed without
	// Commit, no reusable candidate may remain.
	dir := t.TempDir()
	w := newBlobWriter(t, dir, "", -1)
	if _, err := w.Write([]byte("partial data due to network failure")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	assertNoStagingFiles(t, dir)

	if _, err := w.Write([]byte("x")); !errors.Is(err, errBlobWriterDone) {
		t.Errorf("Write after Close error = %v, want errBlobWriterDone", err)
	}
}

func TestCommitFsyncFailure(t *testing.T) {
	dir := t.TempDir()
	content := []byte("fsync is going to fail")
	expectedDigest := digest.FromBytes(content)

	w := newBlobWriter(t, dir, expectedDigest, int64(len(content)))
	injected := errors.New("injected fsync failure")
	w.syncFile = func(*os.File) error { return injected }
	if _, err := w.Write(content); err != nil {
		t.Fatalf("Write: %v", err)
	}
	_, _, err := w.Commit()
	if !errors.Is(err, injected) {
		t.Fatalf("Commit error = %v, want injected fsync error", err)
	}
	assertNoStagingFiles(t, dir)
	if _, err := os.Stat(filepath.Join(dir, expectedDigest.Encoded())); !os.IsNotExist(err) {
		t.Errorf("blob became a candidate despite fsync failure: %v", err)
	}
}

func TestRetryWritersAreIsolated(t *testing.T) {
	dir := t.TempDir()
	content := []byte("the complete retry contents")
	expectedDigest := digest.FromBytes(content)

	// First attempt fails with a short write.
	failing := newBlobWriter(t, dir, expectedDigest, int64(len(content)))
	if _, err := failing.Write(content[:len(content)-3]); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if _, _, err := failing.Commit(); !errors.Is(err, ErrSizeMismatch) {
		t.Fatalf("failing Commit error = %v, want ErrSizeMismatch", err)
	}

	// Independent second attempt succeeds using its own staging file.
	retry := newBlobWriter(t, dir, expectedDigest, int64(len(content)))
	if _, err := io.Copy(retry, bytes.NewReader(content)); err != nil {
		t.Fatalf("retry Write: %v", err)
	}
	gotDigest, gotSize, err := retry.Commit()
	if err != nil {
		t.Fatalf("retry Commit: %v", err)
	}
	if gotDigest != expectedDigest || gotSize != int64(len(content)) {
		t.Errorf("retry got (%q, %d), want (%q, %d)", gotDigest, gotSize, expectedDigest, len(content))
	}
	assertNoStagingFiles(t, dir)
}

func TestConfirmedBlobNotDeletedByFailingWriter(t *testing.T) {
	dir := t.TempDir()
	content := []byte("already confirmed, leave me alone")
	expectedDigest := digest.FromBytes(content)
	finalPath := filepath.Join(dir, expectedDigest.Encoded())

	// Publish a confirmed blob.
	first := newBlobWriter(t, dir, expectedDigest, int64(len(content)))
	if _, err := first.Write(content); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if _, _, err := first.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	original, err := os.ReadFile(finalPath)
	if err != nil {
		t.Fatalf("reading confirmed blob: %v", err)
	}

	// A writer with wrong contents for the same declared digest must fail and
	// leave the confirmed blob in place.
	wrong := newBlobWriter(t, dir, expectedDigest, int64(len(content)))
	if _, err := wrong.Write([]byte("totally different contents here!!")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if _, _, err := wrong.Commit(); !errors.Is(err, ErrDigestMismatch) {
		t.Fatalf("wrong writer Commit error = %v, want ErrDigestMismatch", err)
	}

	// A short-write writer for the same declared digest must also fail and
	// leave the confirmed blob in place.
	short := newBlobWriter(t, dir, expectedDigest, int64(len(content)))
	if _, err := short.Write(content[:4]); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if _, _, err := short.Commit(); !errors.Is(err, ErrSizeMismatch) {
		t.Fatalf("short writer Commit error = %v, want ErrSizeMismatch", err)
	}

	after, err := os.ReadFile(finalPath)
	if err != nil {
		t.Fatalf("confirmed blob disappeared: %v", err)
	}
	if !bytes.Equal(after, original) {
		t.Errorf("confirmed blob contents changed: %q, want %q", after, original)
	}
	assertNoStagingFiles(t, dir)
}

func TestCommitIdempotentWithIdenticalWriter(t *testing.T) {
	dir := t.TempDir()
	content := []byte("published twice is fine")
	expectedDigest := digest.FromBytes(content)

	for i := 0; i < 2; i++ {
		w := newBlobWriter(t, dir, expectedDigest, int64(len(content)))
		if _, err := w.Write(content); err != nil {
			t.Fatalf("attempt %d Write: %v", i, err)
		}
		gotDigest, gotSize, err := w.Commit()
		if err != nil {
			t.Fatalf("attempt %d Commit: %v", i, err)
		}
		if gotDigest != expectedDigest || gotSize != int64(len(content)) {
			t.Errorf("attempt %d got (%q, %d)", i, gotDigest, gotSize)
		}
	}
	assertNoStagingFiles(t, dir)
}

func TestPublishLeavesConflictingWrongBlobUntouched(t *testing.T) {
	dir := t.TempDir()
	content := []byte("1234567890") // 10 bytes
	expectedDigest := digest.FromBytes(content)

	// Pre-create a different file at the final path (same size, different
	// contents), as if it were an externally confirmed blob.
	foreign := []byte("abcdefghij")
	if err := os.WriteFile(filepath.Join(dir, expectedDigest.Encoded()), foreign, 0644); err != nil {
		t.Fatalf("planting foreign blob: %v", err)
	}

	w := newBlobWriter(t, dir, expectedDigest, int64(len(content)))
	if _, err := w.Write(content); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if _, _, err := w.Commit(); !errors.Is(err, ErrDigestMismatch) {
		t.Fatalf("Commit error = %v, want ErrDigestMismatch", err)
	}
	// The conflicting file must not have been removed.
	kept, err := os.ReadFile(filepath.Join(dir, expectedDigest.Encoded()))
	if err != nil {
		t.Fatalf("conflicting final blob disappeared: %v", err)
	}
	if !bytes.Equal(kept, foreign) {
		t.Errorf("conflicting final blob modified: %q, want %q", kept, foreign)
	}
	assertNoStagingFiles(t, dir)
}

func TestSeekAndRewrite(t *testing.T) {
	dir := t.TempDir()
	content := []byte("seek me back and rewrite everything here")
	expectedDigest := digest.FromBytes(content)

	w := newBlobWriter(t, dir, expectedDigest, int64(len(content)))
	// Write garbage first, seek back, then overwrite with the real contents.
	garbage := bytes.Repeat([]byte("x"), len(content))
	if _, err := w.Write(garbage); err != nil {
		t.Fatalf("Write garbage: %v", err)
	}
	if _, err := w.Seek(0, io.SeekStart); err != nil {
		t.Fatalf("Seek: %v", err)
	}
	if _, err := w.Write(content); err != nil {
		t.Fatalf("Write after seek: %v", err)
	}
	gotDigest, _, err := w.Commit()
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if gotDigest != expectedDigest {
		t.Errorf("digest = %q, want %q", gotDigest, expectedDigest)
	}
}

func TestWriteAtChunkAssembly(t *testing.T) {
	dir := t.TempDir()
	content := []byte("assembled out of independent chunks!")
	expectedDigest := digest.FromBytes(content)

	w := newBlobWriter(t, dir, expectedDigest, int64(len(content)))
	// Assemble the blob using chunks in non-sequential order.
	chunks := []struct {
		offset int
		data   []byte
	}{
		{10, content[10:20]},
		{20, content[20:]},
		{0, content[0:10]},
	}
	for _, c := range chunks {
		if _, err := w.WriteAt(c.data, int64(c.offset)); err != nil {
			t.Fatalf("WriteAt(%d): %v", c.offset, err)
		}
	}
	gotDigest, gotSize, err := w.Commit()
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if gotDigest != expectedDigest || gotSize != int64(len(content)) {
		t.Errorf("got (%q, %d), want (%q, %d)", gotDigest, gotSize, expectedDigest, len(content))
	}
}

func TestWriteAtRejectsContentBeyondDeclaredSize(t *testing.T) {
	dir := t.TempDir()
	w := newBlobWriter(t, dir, "", 8)
	if _, err := w.WriteAt([]byte("toolong"), 4); !errors.Is(err, ErrTrailingData) {
		t.Fatalf("WriteAt beyond declared size error = %v, want ErrTrailingData", err)
	}
}

func TestCleanupStaging(t *testing.T) {
	dir := t.TempDir()

	// Stale unconfirmed file from a crashed writer.
	stale, err := os.CreateTemp(dir, stagingPrefix+"*")
	if err != nil {
		t.Fatalf("creating stale file: %v", err)
	}
	stale.WriteString("incomplete")
	stale.Close()

	// Unrelated file that must survive.
	unrelated := filepath.Join(dir, "real-blob")
	if err := os.WriteFile(unrelated, []byte("keep me"), 0644); err != nil {
		t.Fatalf("writing unrelated file: %v", err)
	}

	if err := CleanupStaging(dir); err != nil {
		t.Fatalf("CleanupStaging: %v", err)
	}
	if _, err := os.Stat(stale.Name()); !os.IsNotExist(err) {
		t.Errorf("stale staging file not removed: %v", err)
	}
	if _, err := os.Stat(unrelated); err != nil {
		t.Errorf("unrelated file disappeared: %v", err)
	}
}

func TestCleanupStagingDoesNotRemoveActiveWriterFile(t *testing.T) {
	// The shared lock held by an active writer blocks the exclusive cleanup
	// lock; verify cleanup completes (after the writer abandons) without
	// having touched the live staging file.
	dir := t.TempDir()
	w := newBlobWriter(t, dir, "", -1)
	if _, err := w.Write([]byte("actively writing")); err != nil {
		t.Fatalf("Write: %v", err)
	}

	cleanupDone := make(chan error, 1)
	go func() {
		cleanupDone <- CleanupStaging(dir)
	}()

	select {
	case err := <-cleanupDone:
		t.Fatalf("CleanupStaging returned while writer was active: %v", err)
	default: // expected to block on the exclusive lock
	}

	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := <-cleanupDone; err != nil {
		t.Fatalf("CleanupStaging after writer close: %v", err)
	}
	assertNoStagingFiles(t, dir)
}

func TestNewRejectsInvalidOptions(t *testing.T) {
	dir := t.TempDir()
	if _, err := New(dir, nil); err == nil {
		t.Error("New with nil options unexpectedly succeeded")
	}
	if _, err := New(dir, &Options{ExpectedSize: -2}); err == nil {
		t.Error("New with invalid size unexpectedly succeeded")
	}
	if _, err := New(dir, &Options{ExpectedDigest: "not-a-digest"}); err == nil {
		t.Error("New with invalid digest unexpectedly succeeded")
	}
	missing := filepath.Join(dir, "does-not-exist")
	if _, err := New(missing, &Options{FinalPath: finalPathInDir(missing)}); err == nil {
		t.Error("New with missing staging directory unexpectedly succeeded")
	}
}

func TestNonCanonicalExpectedDigest(t *testing.T) {
	dir := t.TempDir()
	content := []byte("digest declared with a non-canonical algorithm")
	canonical := digest.FromBytes(content)
	// A valid digest using a non-canonical algorithm; it is accepted, and the
	// returned identity is the canonical digest computed by the writer.
	nonCanonical := digest.SHA512.FromBytes(content)

	w := newBlobWriter(t, dir, nonCanonical, int64(len(content)))
	if _, err := w.Write(content); err != nil {
		t.Fatalf("Write: %v", err)
	}
	gotDigest, gotSize, err := w.Commit()
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if gotDigest != canonical {
		t.Errorf("digest = %q, want canonical %q", gotDigest, canonical)
	}
	if gotSize != int64(len(content)) {
		t.Errorf("size = %d, want %d", gotSize, len(content))
	}
}
