package signature

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"

	// Register the OCI and SIF transports, so that their scopes can be validated
	// (docker and directory are already imported by other tests in this package).
	_ "github.com/containers/image/v5/oci/layout"
	_ "github.com/containers/image/v5/sif"
	"github.com/containers/image/v5/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// absFixture returns an absolute path to a file under fixtures/, regardless of
// the test working directory.
func absFixture(t *testing.T, name string) string {
	t.Helper()
	abs, err := filepath.Abs(filepath.Join("fixtures", name))
	require.NoError(t, err)
	_, err = os.Stat(abs)
	require.NoError(t, err)
	return abs
}

// writePolicyFile marshals p and writes it to path.
func writePolicyFile(t *testing.T, path string, p *Policy) {
	t.Helper()
	data, err := marshalPolicy(p)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, data, 0o644))
}

// insecurePolicy returns a minimal policy accepting everything.
func insecurePolicy() *Policy {
	return &Policy{Default: PolicyRequirements{NewPRInsecureAcceptAnything()}}
}

// rejectPolicy returns a minimal policy rejecting everything.
func rejectPolicy() *Policy {
	return &Policy{Default: PolicyRequirements{NewPRReject()}}
}

// markerPolicy carries an easily observable docker scope used by the
// concurrency test to identify which snapshot it holds.
func markerPolicy(marker string) *Policy {
	return &Policy{
		Default: PolicyRequirements{NewPRReject()},
		Transports: map[string]PolicyTransportScopes{
			"docker": {
				"example.com/marker/" + marker: {NewPRInsecureAcceptAnything()},
			},
		},
	}
}

// markerOf returns the marker encoded in a markerPolicy snapshot; it fails the
// test if no (or several) markers are present.
func markerOf(t *testing.T, p *Policy) string {
	t.Helper()
	m, ok := markerOfPolicy(p)
	require.True(t, ok, "policy snapshot has no marker scope")
	return m
}

// markerOfPolicy is like markerOf but does not interact with t, so it is safe
// to call from goroutines.
func markerOfPolicy(p *Policy) (string, bool) {
	const prefix = "example.com/marker/"
	found := ""
	for scope := range p.Transports["docker"] {
		if len(scope) > len(prefix) && scope[:len(prefix)] == prefix {
			if found != "" {
				return "", false // More than one marker = inconsistent snapshot.
			}
			found = scope[len(prefix):]
		}
	}
	return found, found != ""
}

func TestValidatePolicy(t *testing.T) {
	gpgKeyData, err := os.ReadFile(absFixture(t, "public-key.gpg"))
	require.NoError(t, err)
	cosignKeyData, err := os.ReadFile(absFixture(t, "cosign.pub"))
	require.NoError(t, err)

	// Valid fulcio + Rekor configuration using real fixture material.
	fulcio, err := NewPRSigstoreSignedFulcio(
		PRSigstoreSignedFulcioWithCAPath(absFixture(t, "fulcio_v1.crt.pem")),
		PRSigstoreSignedFulcioWithOIDCIssuer("https://github.com/login/oauth"),
		PRSigstoreSignedFulcioWithSubjectEmail("mitr@redhat.com"),
	)
	require.NoError(t, err)
	pki, err := NewPRSigstoreSignedPKI(
		PRSigstoreSignedPKIWithCARootsPath(absFixture(t, "pki_root_crts.pem")),
		PRSigstoreSignedPKIWithCAIntermediatesPath(absFixture(t, "pki_intermediate_crts.pem")),
		PRSigstoreSignedPKIWithSubjectEmail("qiwan@redhat.com"),
	)
	require.NoError(t, err)

	validRequirement := func(req PolicyRequirement) PolicyRequirements {
		return PolicyRequirements{req}
	}

	for _, tc := range []struct {
		name  string
		pol   *Policy
		valid bool
	}{
		// Basics
		{"insecure default", insecurePolicy(), true},
		{"reject default", rejectPolicy(), true},
		{"nil policy", nil, false},
		{"missing default", &Policy{}, false},
		{"empty default requirements", &Policy{Default: PolicyRequirements{}}, false},
		{
			"empty scope requirements",
			&Policy{
				Default: PolicyRequirements{NewPRReject()},
				Transports: map[string]PolicyTransportScopes{
					"docker": {"example.com/repo": PolicyRequirements{}},
				},
			},
			false,
		},
		// signedBy
		{
			"signedBy with valid inline GPG key",
			&Policy{Default: validRequirement(mustNewSignedByKeyData(t, SBKeyTypeGPGKeys, gpgKeyData))},
			true,
		},
		{
			"signedBy with valid GPG key path",
			&Policy{Default: validRequirement(mustNewSignedByKeyPath(t, SBKeyTypeGPGKeys, absFixture(t, "public-key.gpg")))},
			true,
		},
		{
			"signedBy with missing key path",
			&Policy{Default: validRequirement(mustNewSignedByKeyPath(t, SBKeyTypeGPGKeys, "/this/file/does/not/exist.gpg"))},
			false,
		},
		{
			"signedBy with garbage inline key",
			&Policy{Default: validRequirement(mustNewSignedByKeyData(t, SBKeyTypeGPGKeys, []byte("this is not a GPG keyring")))},
			false,
		},
		{
			"signedBy with unimplemented keyType",
			&Policy{Default: validRequirement(mustNewSignedByKeyData(t, SBKeyTypeX509Certificates, gpgKeyData))},
			false,
		},
		{
			"signedBy with signing-GPG keyType",
			&Policy{Default: validRequirement(mustNewSignedByKeyData(t, SBKeyTypeSignedByGPGKeys, gpgKeyData))},
			false,
		},
		// sigstore
		{
			"sigstore with valid inline key",
			&Policy{Default: validRequirement(mustNewSigstoreKeyData(t, cosignKeyData))},
			true,
		},
		{
			"sigstore with valid key path",
			&Policy{Default: validRequirement(mustNewSigstoreKeyPath(t, absFixture(t, "cosign.pub")))},
			true,
		},
		{
			"sigstore with missing key path",
			&Policy{Default: validRequirement(mustNewSigstoreKeyPath(t, "/no/such/cosign.pub"))},
			false,
		},
		{
			"sigstore with garbage inline key",
			&Policy{Default: validRequirement(mustNewSigstoreKeyData(t, []byte("not a PEM public key")))},
			false,
		},
		{
			"sigstore fulcio+rekor valid",
			&Policy{Default: validRequirement(mustNewSigstore(t,
				PRSigstoreSignedWithFulcio(fulcio),
				PRSigstoreSignedWithRekorPublicKeyPath(absFixture(t, "rekor.pub")),
				PRSigstoreSignedWithSignedIdentity(NewPRMMatchRepoDigestOrExact()),
			))},
			true,
		},
		{
			"sigstore PKI valid",
			&Policy{Default: validRequirement(mustNewSigstore(t,
				PRSigstoreSignedWithPKI(pki),
				PRSigstoreSignedWithSignedIdentity(NewPRMMatchRepoDigestOrExact()),
			))},
			true,
		},
		// Transport scopes for docker / oci / sif / unknown transports.
		{
			"docker scopes always accepted",
			&Policy{
				Default: PolicyRequirements{NewPRReject()},
				Transports: map[string]PolicyTransportScopes{
					"docker": {
						"":                        {NewPRInsecureAcceptAnything()},
						"example.com/repo":        {NewPRInsecureAcceptAnything()},
						"registry.example:5000/x": {NewPRInsecureAcceptAnything()},
					},
				},
			},
			true,
		},
		{
			"valid OCI absolute scope",
			&Policy{
				Default: PolicyRequirements{NewPRReject()},
				Transports: map[string]PolicyTransportScopes{
					"oci": {"/srv/oci/layout": {NewPRInsecureAcceptAnything()}},
				},
			},
			true,
		},
		{
			"invalid OCI relative scope",
			&Policy{
				Default: PolicyRequirements{NewPRReject()},
				Transports: map[string]PolicyTransportScopes{
					"oci": {"relative/layout": {NewPRInsecureAcceptAnything()}},
				},
			},
			false,
		},
		{
			"valid SIF absolute scope",
			&Policy{
				Default: PolicyRequirements{NewPRReject()},
				Transports: map[string]PolicyTransportScopes{
					"sif": {"/srv/images/image.sif": {NewPRInsecureAcceptAnything()}},
				},
			},
			true,
		},
		{
			"invalid SIF relative scope",
			&Policy{
				Default: PolicyRequirements{NewPRReject()},
				Transports: map[string]PolicyTransportScopes{
					"sif": {"relative/image.sif": {NewPRInsecureAcceptAnything()}},
				},
			},
			false,
		},
		{
			"SIF root scope rejected",
			&Policy{
				Default: PolicyRequirements{NewPRReject()},
				Transports: map[string]PolicyTransportScopes{
					"sif": {"/": {NewPRInsecureAcceptAnything()}},
				},
			},
			false,
		},
		{
			"unknown transport scope is allowed",
			&Policy{
				Default: PolicyRequirements{NewPRReject()},
				Transports: map[string]PolicyTransportScopes{
					"not-a-registered-transport": {"any/scope/value": {NewPRInsecureAcceptAnything()}},
				},
			},
			true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidatePolicy(tc.pol)
			if tc.valid {
				assert.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.True(t, IsPolicyConfigurationError(err), "expected a policy configuration error, got %T: %v", err, err)
			}
		})
	}
}

func mustNewSignedByKeyPath(t *testing.T, keyType sbKeyType, path string) PolicyRequirement {
	t.Helper()
	req, err := NewPRSignedByKeyPath(keyType, path, NewPRMMatchRepoDigestOrExact())
	require.NoError(t, err)
	return req
}

func mustNewSignedByKeyData(t *testing.T, keyType sbKeyType, data []byte) PolicyRequirement {
	t.Helper()
	req, err := NewPRSignedByKeyData(keyType, data, NewPRMMatchRepoDigestOrExact())
	require.NoError(t, err)
	return req
}

func mustNewSigstoreKeyPath(t *testing.T, path string) PolicyRequirement {
	t.Helper()
	req, err := NewPRSigstoreSignedKeyPath(path, NewPRMMatchRepoDigestOrExact())
	require.NoError(t, err)
	return req
}

func mustNewSigstoreKeyData(t *testing.T, data []byte) PolicyRequirement {
	t.Helper()
	req, err := NewPRSigstoreSignedKeyData(data, NewPRMMatchRepoDigestOrExact())
	require.NoError(t, err)
	return req
}

func mustNewSigstore(t *testing.T, opts ...PRSigstoreSignedOption) PolicyRequirement {
	t.Helper()
	req, err := NewPRSigstoreSigned(opts...)
	require.NoError(t, err)
	return req
}

func TestPolicyStoreLoad(t *testing.T) {
	t.Run("valid primary", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "policy.json")
		writePolicyFile(t, path, insecurePolicy())

		s, err := NewPolicyStore(path)
		require.NoError(t, err)
		require.NotNil(t, s.Policy())
		assert.Len(t, s.Policy().Default, 1)
	})

	t.Run("missing primary", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "policy.json")
		s, err := NewPolicyStore(path)
		assert.Error(t, err)
		assert.Nil(t, s)
		assert.True(t, os.IsNotExist(err), "expected a not-exist error, got %v", err)
		// A missing policy is not a *broken* configuration.
		assert.False(t, IsPolicyConfigurationError(err))
	})

	t.Run("invalid JSON primary", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "policy.json")
		require.NoError(t, os.WriteFile(path, []byte(`{ "default": `), 0o644))
		s, err := NewPolicyStore(path)
		require.Error(t, err)
		assert.Nil(t, s)
		assert.True(t, IsPolicyConfigurationError(err))
	})

	t.Run("valid JSON but unusable mechanism", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "policy.json")
		writePolicyFile(t, path, &Policy{
			Default: PolicyRequirements{mustNewSignedByKeyPath(t, SBKeyTypeGPGKeys, "/no/such/key.gpg")},
		})
		s, err := NewPolicyStore(path)
		require.Error(t, err)
		assert.Nil(t, s)
		assert.True(t, IsPolicyConfigurationError(err))
	})

	t.Run("stale candidates removed at startup", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "policy.json")
		writePolicyFile(t, path, insecurePolicy())
		// Simulate processes that died while writing candidates.
		require.NoError(t, os.WriteFile(filepath.Join(dir, ".policy.json.candidate-123"), []byte("{broken"), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(dir, ".policy.json.candidate-abc"), []byte("{}"), 0o644))
		// Unrelated files must be left alone.
		unrelated := filepath.Join(dir, "unrelated-file")
		require.NoError(t, os.WriteFile(unrelated, []byte("keep"), 0o644))

		_, err := NewPolicyStore(path)
		require.NoError(t, err)

		left, err := filepath.Glob(filepath.Join(dir, ".policy.json.candidate-*"))
		require.NoError(t, err)
		assert.Empty(t, left)
		_, err = os.Stat(unrelated)
		assert.NoError(t, err)
	})
}

func TestPolicyStoreUpdate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "policy.json")
	writePolicyFile(t, path, rejectPolicy())
	s, err := NewPolicyStore(path)
	require.NoError(t, err)
	require.Len(t, s.Policy().Default, 1)
	_, isReject := s.Policy().Default[0].(*prReject)
	require.True(t, isReject)

	t.Run("valid update switches atomically and leaves no candidate", func(t *testing.T) {
		require.NoError(t, s.Update(insecurePolicy()))

		// The in-memory read path switched exactly once.
		_, isInsecure := s.Policy().Default[0].(*prInsecureAcceptAnything)
		assert.True(t, isInsecure)

		// The on-disk file is the new, independently parseable policy.
		onDisk, err := NewPolicyFromFile(path)
		require.NoError(t, err)
		_, onDiskInsecure := onDisk.Default[0].(*prInsecureAcceptAnything)
		assert.True(t, onDiskInsecure)

		left, err := filepath.Glob(filepath.Join(dir, ".policy.json.candidate-*"))
		require.NoError(t, err)
		assert.Empty(t, left)
	})

	t.Run("GPG keyPath policy commits", func(t *testing.T) {
		p := &Policy{Default: PolicyRequirements{mustNewSignedByKeyPath(t, SBKeyTypeGPGKeys, absFixture(t, "public-key.gpg"))}}
		require.NoError(t, s.Update(p))
		assert.Len(t, s.Policy().Default, 1)
	})

	// Every failed update below must leave both the served snapshot and the
	// on-disk primary untouched.
	failureCases := []struct {
		name string
		data []byte
	}{
		{
			"broken JSON",
			[]byte(`{ "default": `),
		},
		{
			"missing default section",
			[]byte(`{}`),
		},
		{
			"invalid docker reference in signedIdentity",
			[]byte(`{
				"default": [
					{"type": "signedBy", "keyType": "GPGKeys",
					 "keyData": "YWJj",
					 "signedIdentity": {"type": "exactReference", "dockerReference": "INVALID UPPERCASE:latest"}}
				]
			}`),
		},
		{
			"invalid SIF scope",
			[]byte(`{
				"default": [{"type": "reject"}],
				"transports": {
					"sif": {"relative/image.sif": [{"type": "insecureAcceptAnything"}]}
				}
			}`),
		},
		{
			"signedBy missing key file",
			[]byte(`{
				"default": [
					{"type": "signedBy", "keyType": "GPGKeys", "keyPath": "/this/file/does/not/exist.gpg"}
				]
			}`),
		},
		{
			"sigstore garbage key",
			[]byte(`{
				"default": [
					{"type": "sigstoreSigned", "keyData": "bm90LWEta2V5"}
				]
			}`),
		},
		{
			"unimplemented keyType",
			[]byte(`{
				"default": [
					{"type": "signedBy", "keyType": "X.509Certificates", "keyData": "YWJj"}
				]
			}`),
		},
	}
	for _, tc := range failureCases {
		t.Run("invalid candidate keeps last valid policy: "+tc.name, func(t *testing.T) {
			before, err := os.ReadFile(path)
			require.NoError(t, err)
			beforeSnap := s.Policy()

			// The failure is a configuration error, not a signature rejection.
			err = s.UpdateFromBytes(tc.data)
			require.Error(t, err)
			assert.True(t, IsPolicyConfigurationError(err), "%T: %v", err, err)
			assert.False(t, IsPolicyRequirementError(err))

			// Served snapshot is still the last valid policy.
			assert.Same(t, beforeSnap, s.Policy())
			// Primary file untouched.
			after, err := os.ReadFile(path)
			require.NoError(t, err)
			assert.Equal(t, before, after)
			// No candidate left behind.
			left, err := filepath.Glob(filepath.Join(dir, ".policy.json.candidate-*"))
			require.NoError(t, err)
			assert.Empty(t, left)
		})
	}

	t.Run("write failure keeps last valid policy", func(t *testing.T) {
		// A different store whose target directory is blocked by a regular file.
		blockedDir := filepath.Join(t.TempDir(), "sub")
		blockedPath := filepath.Join(blockedDir, "policy.json")
		require.NoError(t, os.MkdirAll(blockedDir, 0o755))
		writePolicyFile(t, blockedPath, rejectPolicy())
		blockedStore, err := NewPolicyStore(blockedPath)
		require.NoError(t, err)

		// Replace the directory with a regular file; MkdirAll/CreateTemp must fail.
		require.NoError(t, os.RemoveAll(blockedDir))
		require.NoError(t, os.WriteFile(blockedDir, []byte("i am a file, not a directory"), 0o644))

		err = blockedStore.Update(insecurePolicy())
		require.Error(t, err)
		assert.False(t, IsPolicyConfigurationError(err), "I/O failures are not configuration errors")
		_, stillReject := blockedStore.Policy().Default[0].(*prReject)
		assert.True(t, stillReject)
	})
}

func TestPolicyStoreSnapshotConsistency(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "policy.json")
	writePolicyFile(t, path, markerPolicy("zero"))
	s, err := NewPolicyStore(path)
	require.NoError(t, err)
	assert.Equal(t, "zero", markerOf(t, s.Policy()))

	// A snapshot pinned before an update keeps evaluating against the old
	// policy even after the read path switched.
	pinned := s.Policy()
	require.NoError(t, s.Update(markerPolicy("one")))
	require.NoError(t, s.Update(markerPolicy("two")))
	assert.Equal(t, "zero", markerOf(t, pinned), "pinned snapshot must not change")
	assert.Equal(t, "two", markerOf(t, s.Policy()), "new reads observe the new policy")

	// A PolicyContext created from the pinned snapshot still works.
	pc, err := NewPolicyContext(pinned)
	require.NoError(t, err)
	defer func() { assert.NoError(t, pc.Destroy()) }()
	assert.Same(t, pinned, pc.Policy)
}

func TestPolicyStoreConcurrent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "policy.json")
	writePolicyFile(t, path, markerPolicy("m0"))
	s, err := NewPolicyStore(path)
	require.NoError(t, err)

	const iterations = 200
	var wg sync.WaitGroup
	stop := make(chan struct{})

	// Readers: every acquired snapshot must be internally consistent (one
	// marker), both during and after updates.
	for r := 0; r < 8; r++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				snap := s.Policy()
				if snap == nil {
					t.Error("nil policy snapshot")
					return
				}
				// Re-reading the same snapshot repeatedly must always return
				// the same marker; the snapshot must never be mutated in place.
				m, ok := markerOfPolicy(snap)
				if !ok {
					t.Error("inconsistent policy snapshot")
					return
				}
				for i := 0; i < 10; i++ {
					if m2, ok := markerOfPolicy(snap); !ok || m2 != m {
						t.Error("policy snapshot changed in place")
						return
					}
				}
			}
		}()
	}

	// Updater: valid marker updates alternate with intentionally invalid
	// candidates; the latter must never displace the last valid policy.
	var lastValid *Policy
	for i := 1; i <= iterations; i++ {
		var updateErr error
		if i%5 == 0 {
			updateErr = s.UpdateFromBytes([]byte(`{ "default": [{"type": "no-such-requirement"}]}`))
			require.Error(t, updateErr)
			assert.True(t, IsPolicyConfigurationError(updateErr))
			assert.Same(t, lastValid, s.Policy(), "invalid candidate displaced the active policy")
		} else {
			next := markerPolicy("m" + itoa(i))
			updateErr = s.Update(next)
			require.NoError(t, updateErr)
			lastValid = next
			assert.Same(t, next, s.Policy())
		}
	}
	close(stop)
	wg.Wait()
}

// itoa avoids importing strconv just for marker naming.
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	return string(buf[pos:])
}

func TestPolicyStoreErrorClassification(t *testing.T) {
	assert.False(t, IsPolicyConfigurationError(nil))
	assert.False(t, IsPolicyRequirementError(nil))

	// A signature rejection decided by a valid policy.
	rejection := PolicyRequirementError("A signature was required, but no signature exists")
	assert.True(t, IsPolicyRequirementError(error(rejection)))
	assert.False(t, IsPolicyConfigurationError(error(rejection)))

	// Parse-time JSON structure errors are configuration errors as well.
	_, err := NewPolicyFromBytes([]byte(`{"default":[{"type":"unknown"}]}`))
	require.Error(t, err)
	assert.True(t, IsPolicyConfigurationError(err))
	assert.False(t, IsPolicyRequirementError(err))

	// Strict-parser errors (internal JSONFormatError, e.g. a duplicate key).
	_, err = NewPolicyFromBytes([]byte(`{"default":[{"type":"reject"}],"default":[{"type":"reject"}]}`))
	require.Error(t, err)
	assert.True(t, IsPolicyConfigurationError(err))
}

func TestPolicyStorePolicyContextDistinguishesRejection(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "policy.json")
	writePolicyFile(t, path, rejectPolicy())
	s, err := NewPolicyStore(path)
	require.NoError(t, err)

	pc, err := s.NewPolicyContext()
	require.NoError(t, err)
	defer func() { assert.NoError(t, pc.Destroy()) }()

	image := pcImageMock(t, "fixtures/dir-img-unsigned", "testing/manifest:latest")
	allowed, err := pc.IsRunningImageAllowed(context.Background(), image)
	assert.False(t, allowed)
	require.Error(t, err)
	// A valid reject policy yields a signature/policy requirement error …
	assert.True(t, IsPolicyRequirementError(err), "%T: %v", err, err)
	// … which must not be mistaken for a broken configuration.
	assert.False(t, IsPolicyConfigurationError(err))

	// After switching to an accepting policy (via the validated, atomic path),
	// the very same image is allowed.
	require.NoError(t, s.Update(insecurePolicy()))
	pc2, err := s.NewPolicyContext()
	require.NoError(t, err)
	defer func() { assert.NoError(t, pc2.Destroy()) }()
	allowed, err = pc2.IsRunningImageAllowed(context.Background(), image)
	require.NoError(t, err)
	assert.True(t, allowed)
}

func TestNewDefaultPolicyStorePathResolution(t *testing.T) {
	// No policy configured: the store reports the same kind of failure as
	// DefaultPolicy, instead of substituting a fabricated fallback policy.
	missingPath := filepath.Join(t.TempDir(), "policy.json")
	_, directErr := DefaultPolicy(&types.SystemContext{SignaturePolicyPath: missingPath})
	require.Error(t, directErr)

	s, err := NewDefaultPolicyStore(&types.SystemContext{SignaturePolicyPath: missingPath})
	assert.Error(t, err)
	assert.Nil(t, s)
	assert.True(t, os.IsNotExist(err), "expected a not-exist error, got %v", err)

	// A configured policy path is loaded and validated like in NewPolicyStore.
	existing := filepath.Join(t.TempDir(), "policy.json")
	writePolicyFile(t, existing, insecurePolicy())
	s, err = NewDefaultPolicyStore(&types.SystemContext{SignaturePolicyPath: existing})
	require.NoError(t, err)
	assert.NotNil(t, s.Policy())
}
