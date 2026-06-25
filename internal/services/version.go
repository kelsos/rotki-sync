package services

import (
	"fmt"
	"strconv"
	"strings"
)

// LastTestedCoreVersion is the rotki-core version the CLI's endpoint contract
// was last verified against. Bump it whenever the bundled core is upgraded and
// the sync has been re-validated end to end. A core upgrade is exactly what
// removed the legacy EVM transaction endpoints and silently broke the sync, so
// a mismatch here is a loud, deliberate prompt to re-check the contract.
//
// The legacy /blockchains/evm/transactions[/decode] routes were removed in
// rotki-core v1.41.0 (commit ac1212bcb8 "Generalize tx decoding endpoints");
// the unified /blockchains/transactions fetch route predates even that. The
// gate compares on major.minor, so any minor jump is flagged for re-check.
const LastTestedCoreVersion = "1.43.2"

// semver holds a parsed major.minor.patch version. Pre-release/build suffixes
// are ignored.
type semver struct {
	major, minor, patch int
}

// parseSemver parses a "1.43.2" style version, tolerating a leading "v" and a
// trailing "-suffix" or "+build".
func parseSemver(v string) (semver, error) {
	s := strings.TrimSpace(v)
	s = strings.TrimPrefix(s, "v")
	if i := strings.IndexAny(s, "-+"); i >= 0 {
		s = s[:i]
	}
	parts := strings.Split(s, ".")
	if len(parts) < 2 {
		return semver{}, fmt.Errorf("not a semantic version: %q", v)
	}

	var out semver
	fields := []*int{&out.major, &out.minor, &out.patch}
	for i := 0; i < len(parts) && i < len(fields); i++ {
		n, err := strconv.Atoi(parts[i])
		if err != nil {
			return semver{}, fmt.Errorf("invalid version component %q in %q", parts[i], v)
		}
		*fields[i] = n
	}
	return out, nil
}

// CoreVersionStatus describes how the running rotki-core version compares to the
// last-tested one.
type CoreVersionStatus struct {
	Running    string
	Tested     string
	Compatible bool
	// Warning is a non-empty, human-readable explanation when the running
	// version is not known-compatible.
	Warning string
}

// CheckCoreVersion compares the running rotki-core version against the
// last-tested one. Versions are considered compatible when their major and
// minor components match (patch releases are assumed contract-stable). Any
// other case is flagged with a warning rather than blocking the run.
func CheckCoreVersion(running string) CoreVersionStatus {
	status := CoreVersionStatus{Running: running, Tested: LastTestedCoreVersion}

	got, err := parseSemver(running)
	if err != nil {
		status.Warning = fmt.Sprintf("could not parse rotki-core version %q; endpoint contract is untested", running)
		return status
	}
	want, err := parseSemver(LastTestedCoreVersion)
	if err != nil {
		// Misconfigured constant; do not block the run over it.
		status.Compatible = true
		return status
	}

	if got.major == want.major && got.minor == want.minor {
		status.Compatible = true
		return status
	}

	rel := "is older than"
	if got.major > want.major || (got.major == want.major && got.minor > want.minor) {
		rel = "is newer than"
	}
	status.Warning = fmt.Sprintf(
		"running rotki-core %s %s the last-tested %s; the transaction endpoint contract may have changed",
		running, rel, LastTestedCoreVersion)
	return status
}
