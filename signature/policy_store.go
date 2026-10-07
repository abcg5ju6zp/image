// policy_store.go provides a managed, atomically-updatable signature policy.
//
// A naïve workflow, where the administrator replaces policy.json in place and
// every pull re-reads the file (or a cache is invalidated mid-update), has two
// races:
//
//   - The file can be observed half-written, or the in-memory cache can switch
//     in the middle of a pull, so that different layers/manifests of the same
//     image are evaluated against different policies.
//   - A process restart can load the broken replacement outright and end up
//     rejecting every image.
//
// PolicyStore closes both races: every update is first fully parsed and
// validated (including transport scopes, image references and the signature
// verification mechanisms) as a *candidate* policy; only then is the candidate
// written durably to a temporary file and atomically renamed into place, and
// only after that does the in-memory snapshot served to readers switch once.
// Any failure keeps serving the last successfully committed policy, and
// leftover candidate files of interrupted updates are removed at startup.

package signature

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"github.com/containers/image/v5/signature/internal"
	"github.com/containers/image/v5/transports"
	"github.com/containers/image/v5/types"
)

// candidateTempPrefix is the prefix of temporary candidate files.
// Candidate files always live in the same directory as the primary policy file
// so that committing them is an atomic rename within one filesystem.
func candidateTempPrefix(primaryPath string) string {
	return "." + filepath.Base(primaryPath) + ".candidate-"
}

// candidateTempPattern is the os.CreateTemp/glob pattern of candidate files for primaryPath.
func candidateTempPattern(primaryPath string) string {
	return candidateTempPrefix(primaryPath) + "*"
}

// PolicyStore owns a signature policy file and serves a consistent in-memory
// snapshot of it.
//
// The returned snapshot is never modified in place: successful updates publish
// a new *Policy with a single pointer switch, while pulls that already hold a
// reference (typically through a PolicyContext) keep using the old snapshot
// until they finish. Consequently a single pull can never observe a mix of
// policies, even while an update is in progress.
type PolicyStore struct {
	path string // Primary policy file.
	dir  string // Directory containing the primary file (and candidates).

	// updateMu serializes candidate writes, so that two concurrent updates
	// do not overwrite each other's candidate files or primary file.
	updateMu sync.Mutex

	// currentMu protects current. It is held only for the pointer read/swap,
	// never during disk I/O, so concurrent pulls are not blocked while an
	// update is being validated or written.
	currentMu sync.RWMutex
	current   *Policy // The last successfully loaded/committed policy; never nil for a successfully constructed store.
}

// NewDefaultPolicyStore returns a PolicyStore managing the policy file selected
// by sys (using the same path resolution as DefaultPolicy).
//
// Leftover candidate files of interrupted updates are removed before the
// primary policy is loaded and fully validated; if no policy file is
// configured, the same error as DefaultPolicy is returned.
func NewDefaultPolicyStore(sys *types.SystemContext) (*PolicyStore, error) {
	policyPath, err := defaultPolicyPath(sys)
	if err != nil {
		return nil, err
	}
	return NewPolicyStore(policyPath)
}

// NewPolicyStore returns a PolicyStore managing the policy file at path.
//
// On startup, incomplete candidate files left behind by a process that died
// while updating the policy are removed; afterwards the primary policy is
// loaded and fully validated. A missing or invalid primary file is reported as
// an error instead of silently substituting a fallback (reject-all) policy.
func NewPolicyStore(path string) (*PolicyStore, error) {
	path = filepath.Clean(path)
	s := &PolicyStore{
		path: path,
		dir:  filepath.Dir(path),
	}

	if err := s.removeStaleCandidates(); err != nil {
		return nil, err
	}

	primary, err := s.loadPrimary()
	if err != nil {
		return nil, err
	}
	s.current = primary
	return s, nil
}

// removeStaleCandidates deletes candidate files left over from updates which
// did not complete (typically because the writing process exited before the
// atomic rename).
func (s *PolicyStore) removeStaleCandidates() error {
	matches, err := filepath.Glob(filepath.Join(s.dir, candidateTempPattern(s.path)))
	if err != nil {
		return err
	}
	for _, candidatePath := range matches {
		if err := os.Remove(candidatePath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("cleaning up stale policy candidate %q: %w", candidatePath, err)
		}
	}
	return nil
}

// loadPrimary reads and fully validates the primary policy file.
func (s *PolicyStore) loadPrimary() (*Policy, error) {
	contents, err := os.ReadFile(s.path)
	if err != nil {
		return nil, err
	}
	primary, err := NewPolicyFromBytes(contents)
	if err != nil {
		return nil, fmt.Errorf("invalid policy in %q: %w", s.path, err)
	}
	if err := ValidatePolicy(primary); err != nil {
		return nil, fmt.Errorf("invalid policy in %q: %w", s.path, err)
	}
	return primary, nil
}

// Policy returns the currently active policy snapshot.
//
// The returned *Policy must not be modified. It stays valid and unchanged for
// as long as the caller holds it, even if Update/UpdateFromBytes commits a new
// policy concurrently; to evaluate an image (a pull), create one
// PolicyContext from this snapshot and use it for the whole pull.
func (s *PolicyStore) Policy() *Policy {
	s.currentMu.RLock()
	defer s.currentMu.RUnlock()
	return s.current
}

// NewPolicyContext creates a PolicyContext on the currently active policy
// snapshot. Use the returned context for an entire pull, so that all manifests
// and layers of the image are evaluated against the very same policy.
// The caller must call PolicyContext.Destroy() when done.
func (s *PolicyStore) NewPolicyContext() (*PolicyContext, error) {
	return NewPolicyContext(s.Policy())
}

// UpdateFromBytes parses data as a candidate policy and, only after the
// candidate passes the full validation (syntax, transport scopes/references
// and signature mechanisms), writes it durably, atomically replaces the
// primary file and switches reads to the new policy in one step.
//
// On a parsing, validation, write or rename failure, the primary file and the
// served snapshot stay untouched, i.e. pulls keep using the last valid policy.
// Use IsPolicyConfigurationError to tell such configuration failures apart
// from images being rejected by a valid policy.
func (s *PolicyStore) UpdateFromBytes(data []byte) error {
	candidate, err := NewPolicyFromBytes(data)
	if err != nil {
		return err
	}
	if err := ValidatePolicy(candidate); err != nil {
		return err
	}
	return s.commit(candidate, data)
}

// Update validates a programmatically constructed candidate policy and, on
// success, durably persists and activates it. Semantics on failure are the
// same as for UpdateFromBytes: the previously active policy remains in force.
func (s *PolicyStore) Update(candidate *Policy) error {
	if err := ValidatePolicy(candidate); err != nil {
		return err
	}
	data, err := marshalPolicy(candidate)
	if err != nil {
		return InvalidPolicyFormatError(err.Error())
	}
	return s.commit(candidate, data)
}

// marshalPolicy serializes p into bytes that are guaranteed to parse back into
// an equivalent Policy with NewPolicyFromBytes. In particular, a nil
// Transports map is written as {}, because the strict parser rejects a null
// "transports" value.
func marshalPolicy(p *Policy) ([]byte, error) {
	// Do not modify the caller's Policy; normalize a shallow copy instead.
	normalized := *p
	if normalized.Transports == nil {
		normalized.Transports = map[string]PolicyTransportScopes{}
	}
	return json.MarshalIndent(&normalized, "", "    ")
}

// commit writes data (the already validated candidate) to a temporary file,
// atomically renames it over the primary file, and only then publishes the
// in-memory candidate snapshot.
func (s *PolicyStore) commit(candidate *Policy, data []byte) error {
	s.updateMu.Lock()
	defer s.updateMu.Unlock()

	if err := s.writeCandidate(data); err != nil {
		return err
	}

	// The on-disk switch is complete; switch the read path exactly once.
	s.currentMu.Lock()
	s.current = candidate
	s.currentMu.Unlock()
	return nil
}

// writeCandidate durably writes data to a temporary candidate file in s.dir
// and atomically renames it onto s.path. The temporary file is removed on any
// failure; a successful rename leaves no candidate behind.
func (s *PolicyStore) writeCandidate(data []byte) error {
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return err
	}

	tmp, err := os.CreateTemp(s.dir, candidateTempPattern(s.path))
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	committed := false
	defer func() {
		if !committed {
			// Best effort; ignore a file which is already gone (e.g. the rename succeeded).
			_ = os.Remove(tmpPath)
		}
	}()

	// Preserve the permissions of an existing primary policy file.
	if info, statErr := os.Stat(s.path); statErr == nil {
		_ = tmp.Chmod(info.Mode().Perm())
	}

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}

	if err := os.Rename(tmpPath, s.path); err != nil {
		return err
	}
	committed = true
	// Make the rename durable if the platform supports directory fsync.
	if err := syncDirectory(s.dir); err != nil {
		return fmt.Errorf("syncing policy directory: %w", err)
	}
	return nil
}

// syncDirectory opens dir and fsyncs it, to persist a rename where the
// filesystem supports it. Failures are returned so callers can avoid
// publishing an in-memory policy whose durable commit was not confirmed.
func syncDirectory(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	syncErr := d.Sync()
	closeErr := d.Close()
	if syncErr != nil {
		return syncErr
	}
	return closeErr
}

// ValidatePolicy fully validates a parsed policy candidate:
//
//   - the default requirement set exists and is non-empty;
//   - every transport scope is a valid scope for the transport when that
//     transport is registered in the process (covering docker, oci, sif and
//     the other transports), and every requirement set is non-empty;
//   - every signature requirement is usable: GPG keyrings (signedBy) can be
//     loaded and provide at least one public key; sigstore public keys, Rekor
//     public keys and Fulcio/PKI trust roots can be instantiated;
//     requirement types with no implemented mechanism are rejected.
//
// Unknown transport names (with no registered transport) remain accepted,
// matching the permissive parsing semantics.
//
// Configuration problems are reported as InvalidPolicyFormatError (possibly
// wrapping other errors); this is distinct from PolicyRequirementError, which
// represents an image being rejected by a policy that itself is valid.
func ValidatePolicy(p *Policy) error {
	if p == nil {
		return InvalidPolicyFormatError("policy is nil")
	}
	if len(p.Default) == 0 {
		return InvalidPolicyFormatError("Default policy is missing")
	}
	if err := validateRequirements("default", p.Default); err != nil {
		return err
	}

	transportNames := make([]string, 0, len(p.Transports))
	for name := range p.Transports {
		transportNames = append(transportNames, name)
	}
	sort.Strings(transportNames)
	for _, name := range transportNames {
		scopes := p.Transports[name]
		// A nil transport stands for an unregistered/unknown transport; such
		// transport sections are allowed, just like at parsing time.
		transport := transports.Get(name)
		scopeNames := make([]string, 0, len(scopes))
		for scope := range scopes {
			scopeNames = append(scopeNames, scope)
		}
		sort.Strings(scopeNames)
		for _, scope := range scopeNames {
			reqs := scopes[scope]
			if len(reqs) == 0 {
				return InvalidPolicyFormatError(fmt.Sprintf("List of verification policy requirements must not be empty (transport %q, scope %q)", name, scope))
			}
			if transport != nil && scope != "" {
				if err := transport.ValidatePolicyConfigurationScope(scope); err != nil {
					return InvalidPolicyFormatError(fmt.Sprintf("invalid scope %q for transport %q: %s", scope, name, err.Error()))
				}
			}
			if err := validateRequirements(fmt.Sprintf("transport %q, scope %q", name, scope), reqs); err != nil {
				return err
			}
		}
	}
	return nil
}

// validateRequirements validates that all requirements in reqs are usable.
// location describes where reqs came from, for error messages.
func validateRequirements(location string, reqs PolicyRequirements) error {
	for i, req := range reqs {
		if err := validatePolicyRequirement(req); err != nil {
			return fmt.Errorf("requirement %d of %s: %w", i, location, asPolicyConfigurationError(err))
		}
	}
	return nil
}

// validatePolicyRequirement validates that the signature mechanism of req can
// be instantiated. Requirements without external inputs always succeed.
func validatePolicyRequirement(req PolicyRequirement) error {
	switch r := req.(type) {
	case *prInsecureAcceptAnything, *prReject:
		return nil
	case *prSignedBaseLayer:
		// The base-layer identity has already been validated while parsing;
		// this requirement carries no signature mechanism of its own.
		return nil
	case *prSignedBy:
		return r.validateMechanism()
	case *prSigstoreSigned:
		return r.validateMechanism()
	default:
		// PolicyRequirement implementations are private to this package, so
		// anything else is an internal inconsistency.
		return InvalidPolicyFormatError(fmt.Sprintf("internal error: unsupported policy requirement type %T", req))
	}
}

// validateMechanism verifies that a signedBy requirement can load its trusted
// keys through its signature mechanism.
func (pr *prSignedBy) validateMechanism() error {
	switch pr.KeyType {
	case SBKeyTypeGPGKeys:
		// The supported case: load the configured key material exactly like
		// the evaluation path, and make sure at least one trusted identity
		// can be imported from it.
		data, err := loadBytesFromConfigSources(configBytesSources{
			inconsistencyErrorMessage: `Internal inconsistency: not exactly one of "keyPath", "keyPaths" and "keyData" specified`,
			path:                      pr.KeyPath,
			paths:                     pr.KeyPaths,
			data:                      pr.KeyData,
		})
		if err != nil {
			return InvalidPolicyFormatError(fmt.Sprintf("loading trusted keys for signedBy: %s", err.Error()))
		}
		if len(data) == 0 {
			return InvalidPolicyFormatError(`Internal inconsistency: not exactly one of "keyPath", "keyPaths" and "keyData" specified`)
		}
		mech, trustedIdentities, err := newEphemeralGPGSigningMechanism(data)
		if err != nil {
			return InvalidPolicyFormatError(fmt.Sprintf("initializing GPG signature mechanism for signedBy: %s", err.Error()))
		}
		defer mech.Close()
		if len(trustedIdentities) == 0 {
			return InvalidPolicyFormatError("signedBy policy does not trust any public keys")
		}
		return nil
	case SBKeyTypeSignedByGPGKeys, SBKeyTypeX509Certificates, SBKeyTypeSignedByX509CAs:
		// These keyType values are recognized by the parser but have no
		// verification mechanism; a policy using them could never accept an
		// image, so reject the configuration instead of failing every pull.
		return InvalidPolicyFormatError(fmt.Sprintf(`unsupported "keyType" value %q: no signature verification mechanism is implemented`, pr.KeyType))
	default:
		// newPRSignedBy/parsing ensure KeyType.IsValid(), so this is an internal inconsistency.
		return InvalidPolicyFormatError(fmt.Sprintf(`internal error: unknown "keyType" value %q`, pr.KeyType))
	}
}

// validateMechanism verifies that a sigstoreSigned requirement can build its
// trust root (public keys, Rekor keys, Fulcio/PKI certificates).
func (pr *prSigstoreSigned) validateMechanism() error {
	if _, err := pr.prepareTrustRoot(); err != nil {
		return InvalidPolicyFormatError(fmt.Sprintf("initializing sigstore signature mechanism: %s", err.Error()))
	}
	return nil
}

// asPolicyConfigurationError normalizes errors produced while validating a
// candidate to InvalidPolicyFormatError, so that callers can uniformly
// recognize configuration problems (as opposed to signature rejections).
func asPolicyConfigurationError(err error) error {
	if err == nil {
		return nil
	}
	var invalid InvalidPolicyFormatError
	if errors.As(err, &invalid) {
		return err
	}
	return InvalidPolicyFormatError(err.Error())
}

// IsPolicyConfigurationError reports whether err indicates that the signature
// policy itself is invalid or unusable (a parse/reference/mechanism problem),
// rather than an image being rejected by a valid policy.
func IsPolicyConfigurationError(err error) bool {
	if err == nil {
		return false
	}
	var invalid InvalidPolicyFormatError
	if errors.As(err, &invalid) {
		return true
	}
	var jsonFormat internal.JSONFormatError
	return errors.As(err, &jsonFormat)
}

// IsPolicyRequirementError reports whether err is a decision of a valid policy
// to reject a signature or image (a PolicyRequirementError), as opposed to a
// broken policy configuration or an I/O failure.
func IsPolicyRequirementError(err error) bool {
	if err == nil {
		return false
	}
	var rejected PolicyRequirementError
	return errors.As(err, &rejected)
}
