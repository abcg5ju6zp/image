package docker

import (
	"bufio"
	"bytes"
	"context"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/containers/image/v5/internal/private"
	"github.com/containers/image/v5/internal/signature"
	"github.com/opencontainers/go-digest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var _ private.ImageDestination = (*dockerImageDestination)(nil)

func TestIsManifestInvalidError(t *testing.T) {
	// Sadly only a smoke test; this really should record all known errors exactly as they happen.

	// docker/distribution 2.1.1 when uploading to a tag (because it can’t find a matching tag
	// inside the manifest)
	response := "HTTP/1.1 400 Bad Request\r\n" +
		"Connection: close\r\n" +
		"Content-Length: 79\r\n" +
		"Content-Type: application/json; charset=utf-8\r\n" +
		"Date: Sat, 14 Aug 2021 19:27:29 GMT\r\n" +
		"Docker-Distribution-Api-Version: registry/2.0\r\n" +
		"\r\n" +
		"{\"errors\":[{\"code\":\"TAG_INVALID\",\"message\":\"manifest tag did not match URI\"}]}\n"
	resp, err := http.ReadResponse(bufio.NewReader(bytes.NewReader([]byte(response))), nil)
	require.NoError(t, err)
	defer resp.Body.Close()
	err = registryHTTPResponseToError(resp)

	res := isManifestInvalidError(err)
	assert.True(t, res, "%#v", err)
}

// fakeSimpleSignature returns a SimpleSigning signature whose blob parses as a simple
// signature (it starts with an OpenPGP packet tag byte), with the given distinguishing byte.
func fakeSimpleSignature(distinguishingByte byte) signature.Signature {
	return signature.SimpleSigningFromBlob([]byte{0xA0, distinguishingByte})
}

// readVisibleSignatures mirrors dockerImageSource's lookaside signature enumeration,
// reading through the visible path (following it if it is a symlink).
func readVisibleSignatures(t *testing.T, targetPath string) [][]byte {
	t.Helper()
	var res [][]byte
	for i := 1; ; i++ {
		blob, err := os.ReadFile(filepath.Join(targetPath, "signature-"+strconv.Itoa(i)))
		if err != nil {
			require.True(t, os.IsNotExist(err), err)
			return res
		}
		res = append(res, blob)
	}
}

// requireNoStaleLookasideObjects verifies that no temporary objects exist for baseName in parentDir.
// The set directory currently referenced by the visible path (if it is a symlink) is not stale.
func requireNoStaleLookasideObjects(t *testing.T, parentDir, baseName string) {
	t.Helper()
	visiblePath := filepath.Join(parentDir, baseName)
	liveSet := ""
	if target, err := os.Readlink(visiblePath); err == nil {
		liveSet = filepath.Base(target)
	}
	entries, err := os.ReadDir(parentDir)
	require.NoError(t, err)
	for _, entry := range entries {
		name := entry.Name()
		if name == baseName || name == liveSet {
			continue
		}
		assert.False(t, strings.HasPrefix(name, baseName+".set-"), "unexpected stale set directory %s", name)
		assert.False(t, strings.HasPrefix(name, baseName+".link-"), "unexpected stale link %s", name)
		assert.False(t, strings.HasPrefix(name, baseName+".obsolete-"), "unexpected stale backup %s", name)
	}
}

func newTestLookasideTransaction(t *testing.T) (*lookasideSignatureTransaction, string, string) {
	t.Helper()
	tmpDir := t.TempDir()
	manifestDigest := digest.FromBytes([]byte("lookaside transaction test"))
	base := lookasideStorageBase(&url.URL{Scheme: "file", Path: tmpDir})
	tx, err := newLookasideSignatureTransaction(base, manifestDigest)
	require.NoError(t, err)
	return tx, tmpDir, tx.baseName
}

func TestLookasideSignatureTransactionInitialCommit(t *testing.T) {
	tx, _, baseName := newTestLookasideTransaction(t)

	sigs := []signature.Signature{fakeSimpleSignature(1), fakeSimpleSignature(2)}
	require.NoError(t, tx.stage(context.Background(), sigs))
	// Before the commit, the visible path must not exist.
	_, err := os.Lstat(tx.targetPath)
	require.Error(t, err)
	require.True(t, os.IsNotExist(err))
	require.NoError(t, tx.commit())

	fi, err := os.Lstat(tx.targetPath)
	require.NoError(t, err)
	assert.True(t, fi.Mode()&os.ModeSymlink != 0, "visible path should be a symlink")

	got := readVisibleSignatures(t, tx.targetPath)
	require.Len(t, got, len(sigs))
	for i, sig := range sigs {
		blob, err := signature.Blob(sig)
		require.NoError(t, err)
		assert.True(t, bytes.Equal(blob, got[i]))
	}
	requireNoStaleLookasideObjects(t, tx.parentDir, baseName)
}

func TestLookasideSignatureTransactionAtomicUpdate(t *testing.T) {
	tx, tmp, baseName := newTestLookasideTransaction(t)

	require.NoError(t, tx.stage(context.Background(), []signature.Signature{
		fakeSimpleSignature(1), fakeSimpleSignature(2),
	}))
	require.NoError(t, tx.commit())

	oldLink, err := os.Readlink(tx.targetPath)
	require.NoError(t, err)
	oldSetPath := filepath.Join(tx.parentDir, oldLink)

	// Update with a smaller, reordered set (the same image, signatures replaced).
	tx2, err := newLookasideSignatureTransaction(lookasideStorageBase(&url.URL{Scheme: "file", Path: tmp}),
		digest.FromBytes([]byte("lookaside transaction test")))
	require.NoError(t, err)
	newSigs := []signature.Signature{fakeSimpleSignature(3)}
	require.NoError(t, tx2.stage(context.Background(), newSigs))
	require.NoError(t, tx2.commit())

	// Readers see exactly the new, complete set through the same visible path.
	got := readVisibleSignatures(t, tx2.targetPath)
	require.Len(t, got, 1)
	newBlob, err := signature.Blob(newSigs[0])
	require.NoError(t, err)
	assert.True(t, bytes.Equal(newBlob, got[0]))

	// The superseded set directory has been removed.
	_, err = os.Lstat(oldSetPath)
	assert.True(t, os.IsNotExist(err))
	requireNoStaleLookasideObjects(t, tx.parentDir, baseName)
}

func TestLookasideSignatureTransactionAbortKeepsPreviousSet(t *testing.T) {
	tx, tmp, baseName := newTestLookasideTransaction(t)

	oldSigs := []signature.Signature{fakeSimpleSignature(1), fakeSimpleSignature(2)}
	require.NoError(t, tx.stage(context.Background(), oldSigs))
	require.NoError(t, tx.commit())

	// A canceled context must make the new transaction fail before publishing anything.
	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel()
	tx2, err := newLookasideSignatureTransaction(lookasideStorageBase(&url.URL{Scheme: "file", Path: tmp}),
		digest.FromBytes([]byte("lookaside transaction test")))
	require.NoError(t, err)
	err = tx2.stage(canceledCtx, []signature.Signature{fakeSimpleSignature(3)})
	require.ErrorIs(t, err, context.Canceled)
	tx2.abort()

	// The previous set is still fully visible and verifiable.
	got := readVisibleSignatures(t, tx.targetPath)
	require.Len(t, got, len(oldSigs))
	for i, sig := range oldSigs {
		blob, err := signature.Blob(sig)
		require.NoError(t, err)
		assert.True(t, bytes.Equal(blob, got[i]))
	}
	requireNoStaleLookasideObjects(t, tx.parentDir, baseName)
}

func TestLookasideSignatureTransactionPlainDirMigration(t *testing.T) {
	tx, _, baseName := newTestLookasideTransaction(t)

	// Create a pre-transaction plain directory (as written by older clients),
	// plus stale staging objects of an interrupted earlier attempt.
	require.NoError(t, os.Mkdir(tx.targetPath, 0755))
	oldBlob, err := signature.Blob(fakeSimpleSignature(1))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(tx.targetPath, "signature-1"), oldBlob, 0644))
	staleSet := filepath.Join(tx.parentDir, baseName+".set-stale")
	require.NoError(t, os.Mkdir(staleSet, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(staleSet, "signature-1"), []byte{0}, 0644))
	require.NoError(t, os.Symlink("somewhere", filepath.Join(tx.parentDir, baseName+".link-stale")))

	require.NoError(t, tx.cleanupStaleObjects()) // Removes the stale objects, leaves the plain directory.
	newSigs := []signature.Signature{fakeSimpleSignature(2), fakeSimpleSignature(3)}
	require.NoError(t, tx.stage(context.Background(), newSigs))
	require.NoError(t, tx.commit())

	got := readVisibleSignatures(t, tx.targetPath)
	require.Len(t, got, 2)
	for i, sig := range newSigs {
		blob, err := signature.Blob(sig)
		require.NoError(t, err)
		assert.True(t, bytes.Equal(blob, got[i]))
	}
	requireNoStaleLookasideObjects(t, tx.parentDir, baseName)
}

func TestLookasideSignatureTransactionRestoreObsoleteBackup(t *testing.T) {
	tx, _, baseName := newTestLookasideTransaction(t)

	// Simulate a crash between moving the plain directory aside and publishing the link:
	// the visible path is missing, an obsolete backup and an unpublished set exist.
	backupPath := filepath.Join(tx.parentDir, baseName+".obsolete-backup")
	require.NoError(t, os.Mkdir(backupPath, 0755))
	previousBlob, err := signature.Blob(fakeSimpleSignature(9))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(backupPath, "signature-1"), previousBlob, 0644))
	unpublishedSet := filepath.Join(tx.parentDir, baseName+".set-unpublished")
	require.NoError(t, os.Mkdir(unpublishedSet, 0755))

	require.NoError(t, tx.cleanupStaleObjects())

	// The previous set is restored at the visible path as a plain directory.
	fi, err := os.Lstat(tx.targetPath)
	require.NoError(t, err)
	assert.True(t, fi.IsDir(), "expected the previous plain directory to be restored")
	got := readVisibleSignatures(t, tx.targetPath)
	require.Len(t, got, 1)
	assert.True(t, bytes.Equal(previousBlob, got[0]))
	// The unpublished candidate is removed.
	_, err = os.Lstat(unpublishedSet)
	assert.True(t, os.IsNotExist(err))
}

func TestNewLookasideSignatureTransactionHTTPRejected(t *testing.T) {
	httpURL, err := url.Parse("https://registry.example.com/lookaside")
	require.NoError(t, err)
	_, err = newLookasideSignatureTransaction(lookasideStorageBase(httpURL), digest.FromBytes([]byte("x")))
	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "lookaside-staging"), err.Error())
}
