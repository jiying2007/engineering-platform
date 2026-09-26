package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/jiying2007/engineering-platform/internal/canonical"
	"github.com/jiying2007/engineering-platform/internal/codexexec"
	"github.com/jiying2007/engineering-platform/internal/controlclient"
	"github.com/jiying2007/engineering-platform/internal/core"
	"github.com/jiying2007/engineering-platform/internal/engineeringevidence"
	"github.com/jiying2007/engineering-platform/internal/offline"
	"github.com/jiying2007/engineering-platform/internal/preparation"
)

func offlineReceiptDigest(args []string) error {
	fs := flag.NewFlagSet("offline-receipt-digest", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	runID := fs.String("run", "", "exact Run ID")
	if err := fs.Parse(args); err != nil || fs.NArg() != 0 || strings.TrimSpace(*runID) == "" {
		return fmt.Errorf("usage: eng offline-receipt-digest --run ID")
	}
	control, err := controlclient.FromEnvironment(os.Getenv)
	if err != nil {
		return err
	}
	defer control.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	var status offline.Status
	if err := control.Call(ctx, "GET", "/api/v1/runs/"+*runID+"/offline", nil, &status); err != nil {
		return err
	}
	if status.State != offline.Finished || status.Receipt == nil {
		return fmt.Errorf("Run has no finished offline execution receipt")
	}
	digest, err := engineeringevidence.OfflineReceiptDigest(*status.Receipt)
	if err != nil {
		return err
	}
	printJSON(map[string]any{
		"run_id":          *runID,
		"artifact_digest": digest,
		"media_type":      engineeringevidence.OfflineArtifactType,
	})
	return nil
}

func importOfflineEvidence(args []string) error {
	fs := flag.NewFlagSet("import-offline-evidence", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	deliveryID := fs.String("delivery", "", "exact delivery receipt ID")
	requirementID := fs.String("requirement", "", "exact frozen verification requirement ID")
	evidenceID := fs.String("evidence", "", "new Evidence ID")
	artifactID := fs.String("artifact", "", "Delivery artifact ID bound to the offline receipt digest")
	if err := fs.Parse(args); err != nil || fs.NArg() != 0 {
		return fmt.Errorf("usage: eng import-offline-evidence --delivery ID --requirement ID --evidence ID --artifact ID")
	}
	for _, value := range []string{*deliveryID, *requirementID, *evidenceID, *artifactID} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("all import-offline-evidence flags are required")
		}
	}
	control, err := controlclient.FromEnvironment(os.Getenv)
	if err != nil {
		return err
	}
	defer control.Close()
	if control.Subject() != engineeringevidence.OfflineImporter {
		return fmt.Errorf("dedicated Worker evidence importer mTLS identity required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	var delivery core.DeliveryReceipt
	if err := control.Call(ctx, "GET", "/api/v1/deliveries/"+*deliveryID, nil, &delivery); err != nil {
		return err
	}
	var prep preparation.Receipt
	if err := control.Call(ctx, "GET", "/api/v1/runs/"+delivery.RunID+"/preparation", nil, &prep); err != nil {
		return err
	}
	var status offline.Status
	if err := control.Call(ctx, "GET", "/api/v1/runs/"+delivery.RunID+"/offline", nil, &status); err != nil {
		return err
	}
	evidence, err := engineeringevidence.VerifyOfflineImport(engineeringevidence.OfflineImportRequest{
		EvidenceID: *evidenceID, RequirementID: *requirementID, EvidenceArtifactID: *artifactID,
		Delivery: delivery, Preparation: prep, Execution: status,
	})
	if err != nil {
		return err
	}
	return registerEngineeringEvidence(ctx, control, delivery.ID, evidence)
}

func codexReceiptDigest(args []string) error {
	fs := flag.NewFlagSet("codex-receipt-digest", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	runID := fs.String("run", "", "exact Run ID")
	if err := fs.Parse(args); err != nil || fs.NArg() != 0 || strings.TrimSpace(*runID) == "" {
		return fmt.Errorf("usage: eng codex-receipt-digest --run ID")
	}
	control, err := controlclient.FromEnvironment(os.Getenv)
	if err != nil {
		return err
	}
	defer control.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	var status codexexec.Status
	if err := control.Call(ctx, "GET", "/api/v1/runs/"+*runID+"/codex", nil, &status); err != nil {
		return err
	}
	if status.State != codexexec.Finished || status.Receipt == nil {
		return fmt.Errorf("Run has no finished Core-bound Codex receipt")
	}
	digest, err := engineeringevidence.CodexReceiptDigest(*status.Receipt)
	if err != nil {
		return err
	}
	printJSON(map[string]any{
		"run_id":            *runID,
		"artifact_digest":   digest,
		"media_type":        engineeringevidence.CodexReceiptArtifactType,
		"bundle_digest":     status.Receipt.Result.Change.BundleDigest,
		"bundle_size":       status.Receipt.Result.Change.BundleSize,
		"bundle_media_type": engineeringevidence.CodexBundleArtifactType,
	})
	return nil
}

func importCodexEvidence(args []string) error {
	fs := flag.NewFlagSet("import-codex-evidence", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	deliveryID := fs.String("delivery", "", "exact delivery receipt ID")
	requirementID := fs.String("requirement", "", "exact frozen verification requirement ID")
	evidenceID := fs.String("evidence", "", "new Evidence ID")
	receiptArtifactID := fs.String("receipt-artifact", "", "Delivery artifact ID bound to the Codex receipt digest")
	bundleArtifactID := fs.String("bundle-artifact", "", "Delivery artifact ID bound to the result Git bundle")
	bundlePath := fs.String("bundle", "", "local retained result Git bundle")
	if err := fs.Parse(args); err != nil || fs.NArg() != 0 {
		return fmt.Errorf("usage: eng import-codex-evidence --delivery ID --requirement ID --evidence ID --receipt-artifact ID --bundle-artifact ID --bundle FILE")
	}
	for _, value := range []string{*deliveryID, *requirementID, *evidenceID, *receiptArtifactID, *bundleArtifactID, *bundlePath} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("all import-codex-evidence flags are required")
		}
	}
	control, err := controlclient.FromEnvironment(os.Getenv)
	if err != nil {
		return err
	}
	defer control.Close()
	if control.Subject() != engineeringevidence.CodexImporter {
		return fmt.Errorf("dedicated Codex evidence importer mTLS identity required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var delivery core.DeliveryReceipt
	if err := control.Call(ctx, "GET", "/api/v1/deliveries/"+*deliveryID, nil, &delivery); err != nil {
		return err
	}
	var status codexexec.Status
	if err := control.Call(ctx, "GET", "/api/v1/runs/"+delivery.RunID+"/codex", nil, &status); err != nil {
		return err
	}
	evidence, err := engineeringevidence.VerifyCodexImport(engineeringevidence.CodexImportRequest{
		EvidenceID: *evidenceID, RequirementID: *requirementID,
		ReceiptArtifactID: *receiptArtifactID, BundleArtifactID: *bundleArtifactID,
		Delivery: delivery, Execution: status, BundlePath: *bundlePath,
	})
	if err != nil {
		return err
	}
	return registerEngineeringEvidence(ctx, control, delivery.ID, evidence)
}

func gitChangeManifest(args []string) error {
	fs := flag.NewFlagSet("git-change-manifest", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	repository := fs.String("repository", "", "absolute local Git repository path")
	base := fs.String("base", "", "exact base commit SHA")
	result := fs.String("result", "", "exact result commit SHA")
	output := fs.String("output", "", "new manifest file")
	if err := fs.Parse(args); err != nil || fs.NArg() != 0 {
		return fmt.Errorf("usage: eng git-change-manifest --repository PATH --base SHA --result SHA --output FILE")
	}
	for _, value := range []string{*repository, *base, *result, *output} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("all git-change-manifest flags are required")
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	manifest, err := engineeringevidence.CaptureGitChange(ctx, *repository, strings.ToLower(*base), strings.ToLower(*result))
	if err != nil {
		return err
	}
	data, err := engineeringevidence.MarshalGitChangeManifest(manifest)
	if err != nil {
		return err
	}
	file, err := os.OpenFile(*output, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	_, writeErr := file.Write(append(data, '\n'))
	syncErr := file.Sync()
	closeErr := file.Close()
	if err := firstError(writeErr, syncErr, closeErr); err != nil {
		_ = os.Remove(*output)
		return err
	}
	digest := canonical.BytesDigest(append(data, '\n'))
	printJSON(map[string]any{
		"artifact_digest": digest,
		"media_type":      engineeringevidence.GitArtifactType,
		"manifest":        manifest,
	})
	return nil
}

func importGitChangeEvidence(args []string) error {
	fs := flag.NewFlagSet("import-git-change-evidence", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	repository := fs.String("repository", "", "absolute local Git repository path")
	manifestPath := fs.String("manifest", "", "retained changed-tree manifest file")
	deliveryID := fs.String("delivery", "", "exact delivery receipt ID")
	requirementID := fs.String("requirement", "", "exact frozen verification requirement ID")
	evidenceID := fs.String("evidence", "", "new Evidence ID")
	artifactID := fs.String("artifact", "", "Delivery artifact ID bound to the manifest file digest")
	if err := fs.Parse(args); err != nil || fs.NArg() != 0 {
		return fmt.Errorf("usage: eng import-git-change-evidence --repository PATH --manifest FILE --delivery ID --requirement ID --evidence ID --artifact ID")
	}
	for _, value := range []string{*repository, *manifestPath, *deliveryID, *requirementID, *evidenceID, *artifactID} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("all import-git-change-evidence flags are required")
		}
	}
	retained, fileDigest, err := engineeringevidence.ReadGitChangeManifest(*manifestPath)
	if err != nil {
		return err
	}
	control, err := controlclient.FromEnvironment(os.Getenv)
	if err != nil {
		return err
	}
	defer control.Close()
	if control.Subject() != engineeringevidence.GitImporter {
		return fmt.Errorf("dedicated Git evidence importer mTLS identity required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	var delivery core.DeliveryReceipt
	if err := control.Call(ctx, "GET", "/api/v1/deliveries/"+*deliveryID, nil, &delivery); err != nil {
		return err
	}
	observed, err := engineeringevidence.CaptureGitChange(ctx, *repository, delivery.BaseCommit, delivery.ResultCommit)
	if err != nil {
		return err
	}
	evidence, err := engineeringevidence.VerifyGitImport(engineeringevidence.GitImportRequest{
		EvidenceID: *evidenceID, RequirementID: *requirementID, EvidenceArtifactID: *artifactID,
		Delivery: delivery, Manifest: retained, ManifestFileDigest: fileDigest, Observed: observed,
	})
	if err != nil {
		return err
	}
	return registerEngineeringEvidence(ctx, control, delivery.ID, evidence)
}

func registerEngineeringEvidence(ctx context.Context, control *controlclient.Client, deliveryID string, evidence core.EvidenceRef) error {
	request := struct {
		DeliveryReceiptID string           `json:"delivery_receipt_id"`
		Evidence          core.EvidenceRef `json:"evidence"`
	}{DeliveryReceiptID: deliveryID, Evidence: evidence}
	var stored core.EvidenceRef
	if err := control.Call(ctx, "POST", "/api/v1/evidence", request, &stored); err != nil {
		return err
	}
	if stored.ID != evidence.ID || stored.DeliveryReceiptID != evidence.DeliveryReceiptID ||
		stored.RequirementID != evidence.RequirementID || stored.SubjectDigest != evidence.SubjectDigest ||
		stored.Issuer != evidence.Issuer || stored.Procedure != evidence.Procedure || stored.Result != evidence.Result ||
		stored.Applicable != evidence.Applicable || len(stored.ArtifactRefs) != len(evidence.ArtifactRefs) {
		return fmt.Errorf("control plane returned a different evidence identity")
	}
	for i := range evidence.ArtifactRefs {
		if stored.ArtifactRefs[i] != evidence.ArtifactRefs[i] {
			return fmt.Errorf("control plane returned different evidence artifacts")
		}
	}
	printJSON(stored)
	return nil
}

func firstError(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}
