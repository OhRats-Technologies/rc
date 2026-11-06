package upgrade

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"

	witTypes "go.bytecodealliance.org/pkg/wit/types"
	"wit_component/ohrats_rc_updater_artifact_source"
	"wit_component/ohrats_rc_updater_native_replacement"
)

// maxArtifact bounds the staged native kernel image at 160 MiB.
const maxArtifact = 160 * 1024 * 1024

// Run replaces the native kernel from a pinned artifact. The artifact digest
// must match the expected digest exactly, the candidate must be a newer
// same-major version, and any failure after staging aborts the candidate so
// the running kernel stays active.
func Run(args []string) witTypes.Result[uint32, string] {
	expected, reexec, err := parseArgs(args)
	if err != nil {
		return witTypes.Err[uint32, string](err.Error())
	}
	current := ohrats_rc_updater_native_replacement.Current()
	if current.IsErr() {
		return witTypes.Err[uint32, string](current.Err())
	}
	running := current.Ok()
	currentVersion, err := Parse(running.Version)
	if err != nil {
		return witTypes.Err[uint32, string]("running kernel reported an invalid semantic version")
	}
	handle := ohrats_rc_updater_artifact_source.Select()
	if handle.IsErr() {
		return witTypes.Err[uint32, string](handle.Err())
	}
	artifact := ohrats_rc_updater_artifact_source.Read(handle.Ok())
	if artifact.IsErr() {
		return witTypes.Err[uint32, string](artifact.Err())
	}
	bytes := artifact.Ok()
	if len(bytes) > maxArtifact {
		return witTypes.Err[uint32, string]("kernel artifact exceeds 160 MiB")
	}
	if err := normalizeDigest(expected); err != nil {
		return witTypes.Err[uint32, string](err.Error())
	}
	actual := digest(bytes)
	if actual != expected {
		return witTypes.Err[uint32, string]("kernel artifact digest does not match the pinned digest")
	}
	if running.Digest == expected {
		fmt.Printf("kernel already at %s\n", running.Version)
		return witTypes.Ok[uint32, string](0)
	}
	staged := ohrats_rc_updater_native_replacement.Prepare(bytes, expected)
	if staged.IsErr() {
		return witTypes.Err[uint32, string](staged.Err())
	}
	candidate := staged.Ok()
	target, err := Parse(candidate.Version)
	if err != nil {
		return witTypes.Err[uint32, string]("staged kernel reported an invalid semantic version")
	}
	if err := validateCandidate(currentVersion, target); err != nil {
		ohrats_rc_updater_native_replacement.Abort(candidate)
		return witTypes.Err[uint32, string](err.Error())
	}
	committed := ohrats_rc_updater_native_replacement.Commit(candidate)
	if committed.IsErr() {
		ohrats_rc_updater_native_replacement.Abort(candidate)
		return witTypes.Err[uint32, string](committed.Err())
	}
	if reexec {
		reexecuted := ohrats_rc_updater_native_replacement.Reexec()
		if reexecuted.IsErr() {
			return witTypes.Err[uint32, string](reexecuted.Err())
		}
	}
	fmt.Printf("upgraded kernel to %s\n", target.String())
	return witTypes.Ok[uint32, string](0)
}

// validateCandidate requires a strictly newer version that keeps the running
// major version, so an upgrade never crosses an incompatible kernel ABI.
func validateCandidate(current Version, candidate Version) error {
	if Compare(candidate, current) <= 0 {
		return errors.New("kernel candidate is not newer than the running version")
	}
	if current.Major != candidate.Major {
		return errors.New("kernel candidate is incompatible with the running major version")
	}
	return nil
}

// parseArgs accepts exactly one expected digest plus an optional --reexec
// flag in any position.
func parseArgs(args []string) (string, bool, error) {
	reexec := false
	values := make([]string, 0, len(args))
	for _, arg := range args {
		if arg == "--reexec" {
			reexec = true
			continue
		}
		values = append(values, arg)
	}
	if len(values) != 1 {
		return "", false, errors.New("usage: rc upgrade EXPECTED-DIGEST [--reexec]")
	}
	return values[0], reexec, nil
}

// normalizeDigest enforces strict sha256:<64 lowercase hexadecimal digits>.
func normalizeDigest(value string) error {
	if len(value) != 71 || value[:7] != "sha256:" {
		return errors.New("expected sha256:<64 lowercase hexadecimal digits>")
	}
	for index := 7; index < len(value); index++ {
		byte := value[index]
		if (byte < '0' || byte > '9') && (byte < 'a' || byte > 'f') {
			return errors.New("expected sha256:<64 lowercase hexadecimal digits>")
		}
	}
	return nil
}

func digest(value []byte) string {
	sum := sha256.Sum256(value)
	return "sha256:" + hex.EncodeToString(sum[:])
}
