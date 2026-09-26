package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/jiying2007/engineering-platform/internal/cievidence"
	"github.com/jiying2007/engineering-platform/internal/controlclient"
	"github.com/jiying2007/engineering-platform/internal/core"
)

func importCIEvidence(args []string) error {
	fs := flag.NewFlagSet("import-ci-evidence", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	deliveryID := fs.String("delivery", "", "exact delivery receipt ID")
	requirementID := fs.String("requirement", "", "exact frozen verification requirement ID")
	evidenceID := fs.String("evidence", "", "new evidence ID")
	artifactID := fs.String("artifact", "", "delivery artifact ID that binds the trusted CI envelope ZIP")
	envelopeZip := fs.String("envelope-zip", "", "downloaded trusted-ci-evidence artifact ZIP")
	binariesZip := fs.String("binaries-zip", "", "downloaded engineering-binaries artifact ZIP")
	codexZip := fs.String("codex-zip", "", "downloaded codex qualification artifact ZIP")
	if err := fs.Parse(args); err != nil || fs.NArg() != 0 {
		return fmt.Errorf("usage: eng import-ci-evidence --delivery ID --requirement ID --evidence ID --artifact ID --envelope-zip FILE --binaries-zip FILE --codex-zip FILE")
	}
	for _, value := range []string{*deliveryID, *requirementID, *evidenceID, *artifactID, *envelopeZip, *binariesZip, *codexZip} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("all import-ci-evidence flags are required")
		}
	}
	envelope, err := cievidence.ReadEnvelopeZip(*envelopeZip)
	if err != nil {
		return err
	}
	if envelope.Receipt.Repository != cievidence.TrustedRepository ||
		(envelope.Receipt.Event != "push" && envelope.Receipt.Event != "pull_request") {
		return fmt.Errorf("only trusted main-push or exact PR-head CI evidence can be imported")
	}

	token, err := optionalPrivateToken(os.Getenv("GITHUB_TOKEN_FILE"))
	if err != nil {
		return err
	}
	github, err := cievidence.NewGitHubClient(token)
	if err != nil {
		return err
	}
	defer github.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	live, err := github.FetchLiveFacts(ctx, envelope.Receipt.Repository, envelope.Receipt.RunID)
	if err != nil {
		return err
	}

	control, err := controlclient.FromEnvironment(os.Getenv)
	if err != nil {
		return err
	}
	defer control.Close()
	if control.Subject() != cievidence.ImporterSubject {
		return fmt.Errorf("dedicated trusted CI importer mTLS identity required")
	}
	var delivery core.DeliveryReceipt
	if err := control.Call(ctx, "GET", "/api/v1/deliveries/"+*deliveryID, nil, &delivery); err != nil {
		return err
	}
	evidence, err := cievidence.VerifyTrustedImport(cievidence.ImportRequest{
		EvidenceID:         *evidenceID,
		RequirementID:      *requirementID,
		EvidenceArtifactID: *artifactID,
		Delivery:           delivery,
		Files: cievidence.ImportFiles{
			EnvelopeZip: *envelopeZip,
			BinariesZip: *binariesZip,
			CodexZip:    *codexZip,
		},
		Live: live,
	})
	if err != nil {
		return err
	}
	request := struct {
		DeliveryReceiptID string           `json:"delivery_receipt_id"`
		Evidence          core.EvidenceRef `json:"evidence"`
	}{DeliveryReceiptID: delivery.ID, Evidence: evidence}
	var stored core.EvidenceRef
	if err := control.Call(ctx, "POST", "/api/v1/evidence", request, &stored); err != nil {
		return err
	}
	if stored.ID != evidence.ID || stored.DeliveryReceiptID != evidence.DeliveryReceiptID ||
		stored.RequirementID != evidence.RequirementID || stored.SubjectDigest != evidence.SubjectDigest ||
		stored.Issuer != evidence.Issuer || stored.Procedure != evidence.Procedure || stored.Result != "PASS" ||
		!stored.Applicable || len(stored.ArtifactRefs) != 1 || stored.ArtifactRefs[0] != evidence.ArtifactRefs[0] {
		return fmt.Errorf("control plane returned a different evidence identity")
	}
	printJSON(stored)
	return nil
}

func optionalPrivateToken(path string) (string, error) {
	if path == "" {
		return "", nil
	}
	before, err := os.Lstat(path)
	if err != nil || !before.Mode().IsRegular() || before.Mode().Perm()&0o077 != 0 || before.Size() <= 0 || before.Size() > 4096 {
		return "", fmt.Errorf("GITHUB_TOKEN_FILE must be a bounded owner-private regular file")
	}
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("GITHUB_TOKEN_FILE unavailable")
	}
	defer f.Close()
	after, err := f.Stat()
	if err != nil || !os.SameFile(before, after) {
		return "", fmt.Errorf("GITHUB_TOKEN_FILE changed")
	}
	data, err := io.ReadAll(io.LimitReader(f, 4097))
	if err != nil || len(data) > 4096 {
		return "", fmt.Errorf("GITHUB_TOKEN_FILE read failed or exceeds bound")
	}
	token := strings.TrimSpace(string(data))
	if token == "" || strings.ContainsAny(token, "\r\n") {
		return "", fmt.Errorf("invalid GITHUB_TOKEN_FILE contents")
	}
	return token, nil
}
