// Package preparation binds actual host-side Context/workspace preparation to a
// Worker lease. Its receipt is a Worker attestation, never execution authority.
package preparation

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/jiying2007/engineering-platform/internal/canonical"
	"github.com/jiying2007/engineering-platform/internal/contextbundle"
	"github.com/jiying2007/engineering-platform/internal/core"
	"github.com/jiying2007/engineering-platform/internal/workerqueue"
	"github.com/jiying2007/engineering-platform/internal/workspace"
)

const Kind = "WORKER_ATTESTED_PREPARATION"

var gitSHA = regexp.MustCompile(`^[0-9a-f]{40}$`)

type Facts struct {
	Version          int                    `json:"version"`
	IntentDigest     string                 `json:"intent_digest"`
	InputDigest      string                 `json:"run_input_manifest_digest"`
	TaskDigest       string                 `json:"task_contract_digest"`
	ApprovalDigest   string                 `json:"approval_digest"`
	BaseCommit       string                 `json:"base_commit"`
	TreeCommit       string                 `json:"tree_commit"`
	WorkspaceRecipe  string                 `json:"workspace_recipe"`
	SourceDigest     string                 `json:"source_digest"`
	ConfigDigest     string                 `json:"git_config_digest"`
	BundleDigest     string                 `json:"bundle_digest"`
	Context          contextbundle.Manifest `json:"context_manifest"`
	ExecutionStarted bool                   `json:"execution_started"`
	OSIsolated       bool                   `json:"os_isolated"`
}

// Check re-computes identity/manifest hashes, not remote filesystem bytes. The
// authenticated preparation worker is the attester for the filesystem facts.
func (f Facts) Check(a workerqueue.Assignment) error {
	if _, err := workerqueue.Validate(a); err != nil {
		return err
	}
	if f.Version != 1 || f.ExecutionStarted || f.OSIsolated || f.WorkspaceRecipe != workspace.Recipe || f.BaseCommit != strings.ToLower(a.Task.BaseCommit) || !gitSHA.MatchString(f.TreeCommit) || f.InputDigest != a.Intent.InputDigest || f.TaskDigest != a.Intent.TaskDigest || f.IntentDigest != a.IntentDigest {
		return workerqueue.ErrIdentity
	}
	for _, digest := range []string{f.SourceDigest, f.ConfigDigest, f.BundleDigest, f.ApprovalDigest} {
		if !canonical.ValidDigest(digest) {
			return workerqueue.ErrIdentity
		}
	}
	if f.Context.SchemaVersion != 1 || f.Context.RunInputDigest != a.Intent.InputDigest || len(f.Context.Entries) != len(a.Input.ContextRefs) {
		return workerqueue.ErrIdentity
	}
	var total int64
	for i, ref := range a.Input.ContextRefs {
		entry := f.Context.Entries[i]
		if ref.Trust != core.ContextApproved || entry.Ref != ref || entry.File != fmt.Sprintf("%03d-%s.bin", i, strings.TrimPrefix(ref.Digest, "sha256:")) || entry.Size < 0 || entry.Size > 4<<20 {
			return workerqueue.ErrIdentity
		}
		total += entry.Size
	}
	if total > 32<<20 {
		return workerqueue.ErrIdentity
	}
	raw, err := json.Marshal(f.Context)
	if err != nil || canonical.BytesDigest(raw) != f.BundleDigest {
		return workerqueue.ErrIdentity
	}
	return nil
}

type Report struct {
	Input workerqueue.Report `json:"input_report"`
	Facts Facts              `json:"facts"`
}
type Receipt struct {
	Kind        string              `json:"kind"`
	Admission   workerqueue.Receipt `json:"admission"`
	Facts       Facts               `json:"facts"`
	FactsDigest string              `json:"facts_digest"`
	ReceivedAt  time.Time           `json:"received_at"`
}

func Verify(a workerqueue.Assignment, r Receipt, expected Facts, subject string) error {
	if err := workerqueue.VerifyReceipt(a, r.Admission); err != nil {
		return err
	}
	digest, err := canonical.Digest(expected)
	actual, actualErr := canonical.Digest(r.Facts)
	if err != nil || actualErr != nil || digest != actual || r.FactsDigest != digest || r.Kind != Kind || r.Admission.Worker != subject || r.ReceivedAt.IsZero() {
		return workerqueue.ErrIdentity
	}
	return r.Facts.Check(a)
}

type Repository interface {
	PreparationReady(context.Context) error
	ReportPrepared(context.Context, string, Report) (Receipt, error)
	GetPreparation(context.Context, string) (Receipt, error)
}
