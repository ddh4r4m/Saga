package gate

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"time"

	"github.com/ddh4r4m/saga/internal/canon"
	"github.com/ddh4r4m/saga/internal/schema"
	"github.com/ddh4r4m/saga/internal/store"
)

// Approval is one record under ~/.saga/approved/ (gate-spec section 8).
type Approval struct {
	Schema         string `json:"schema"`
	IdentitySHA256 string `json:"identity_sha256"`
	Gate           string `json:"gate"`
	ApprovedAt     string `json:"approved_at"`
	By             string `json:"by"`
}

// ApprovalEnv names the environment override for the store location.
const ApprovalEnv = "SAGA_APPROVAL_DIR"

// AgentShellMarkers are environment variables a harness sets in the
// agent's shell. approve and attest refuse to run under any of them
// (contracts section 8): those commands are a human act.
var AgentShellMarkers = []string{"SAGA_AGENT_SHELL", "CLAUDECODE", "CLAUDE_CODE_ENTRYPOINT", "CODEX_SANDBOX", "CODEX_CI", "GEMINI_CLI", "CURSOR_AGENT"}

// AgentShell returns the marker that identifies this process as running
// inside an agent's shell, or "".
func AgentShell() string {
	for _, m := range AgentShellMarkers {
		if os.Getenv(m) != "" {
			return m
		}
	}
	return ""
}

// ApprovalDir resolves and validates the approval store: ~/.saga/approved
// by default; SAGA_APPROVAL_DIR only when it is a real, owner-private
// directory whose canonical target is outside the canonical repo root.
// Failures are environment refusals (exit 6).
func ApprovalDir(repoRoot string) (string, error) {
	dir := os.Getenv(ApprovalEnv)
	explicit := dir != ""
	if !explicit {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("approval store: no home directory: %w", err)
		}
		dir = filepath.Join(home, ".saga", "approved")
	}
	if fi, err := os.Lstat(dir); err == nil {
		if fi.Mode()&fs.ModeSymlink != 0 {
			return "", fmt.Errorf("approval store %s is a symlink", dir)
		}
		if !fi.IsDir() {
			return "", fmt.Errorf("approval store %s is not a directory", dir)
		}
		if fi.Mode().Perm()&0o077 != 0 {
			return "", fmt.Errorf("approval store %s is not owner-private (mode %o)", dir, fi.Mode().Perm())
		}
	} else if errors.Is(err, fs.ErrNotExist) {
		if explicit {
			return "", fmt.Errorf("approval store %s does not exist", dir)
		}
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return "", fmt.Errorf("approval store: %w", err)
		}
	} else {
		return "", err
	}
	canonDir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return "", err
	}
	if repoRoot != "" {
		if canonRoot, err := filepath.EvalSymlinks(repoRoot); err == nil {
			if canonDir == canonRoot || strings.HasPrefix(canonDir, canonRoot+string(filepath.Separator)) {
				return "", fmt.Errorf("approval store %s is inside the repository", dir)
			}
		}
	}
	return canonDir, nil
}

// ApprovalIdentity hashes the section 8 identity: repo-relative contract
// path, gate id, oracle_hash, platform, the full inherited PATH and the
// witness_hash. Only the hash is stored.
func ApprovalIdentity(contractPath, gate, oracleHash, platform, pathEnv, witnessHash string) string {
	h, _ := canon.SHA256JSON(map[string]string{
		"contract": contractPath, "gate": gate, "oracle_hash": oracleHash, "platform": platform, "path": pathEnv, "witness_hash": witnessHash,
	})
	return h
}

func approvalFile(dir, identity string) string {
	return filepath.Join(dir, strings.TrimPrefix(identity, "sha256:")+".json")
}

// LoadApproval reads the record for identity; missing is (nil, nil).
func LoadApproval(dir, identity string) (*Approval, error) {
	p := approvalFile(dir, identity)
	if err := store.CheckShape(p); err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(p)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var a Approval
	if err := json.Unmarshal(raw, &a); err != nil || a.Schema != ApprovalSchema || a.IdentitySHA256 != identity {
		return nil, fmt.Errorf("approval record %s is malformed", p)
	}
	return &a, nil
}

// WriteApproval records consent for identity by the invoking OS user.
func WriteApproval(dir, identity, gate string) (*Approval, error) {
	by := "unknown"
	if u, err := user.Current(); err == nil {
		by = u.Username
	}
	a := &Approval{Schema: ApprovalSchema, IdentitySHA256: identity, Gate: gate, ApprovedAt: time.Now().UTC().Format(time.RFC3339), By: by}
	v, err := schema.Normalize(a)
	if err != nil {
		return nil, err
	}
	if err := schema.ValidateID(ApprovalSchema, v); err != nil {
		return nil, err
	}
	raw, err := canon.JSON(a)
	if err != nil {
		return nil, err
	}
	if err := store.WriteFileAtomic(approvalFile(dir, identity), append(raw, '\n'), 0o600); err != nil {
		return nil, err
	}
	return a, nil
}

// RevokeApproval removes the record for identity.
func RevokeApproval(dir, identity string) error {
	err := os.Remove(approvalFile(dir, identity))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}
