// Package cievidence defines retained Git/CI/artifact facts. These facts are
// delivery provenance, not engineering acceptance or an independent review.
package cievidence

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/jiying2007/engineering-platform/internal/canonical"
)

const SchemaVersion = 1

var (
	sha40    = regexp.MustCompile(`^[0-9a-f]{40}$`)
	repoName = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)
)

type Job struct {
	Name       string `json:"name"`
	ID         int64  `json:"id"`
	Conclusion string `json:"conclusion"`
}

type Artifact struct {
	Name   string `json:"name"`
	ID     int64  `json:"id"`
	Digest string `json:"digest"`
	Size   int64  `json:"size"`
}

type File struct {
	Path   string `json:"path"`
	Digest string `json:"digest"`
	Size   int64  `json:"size"`
}

type Receipt struct {
	SchemaVersion int        `json:"schema_version"`
	Repository    string     `json:"repository"`
	Workflow      string     `json:"workflow"`
	Event         string     `json:"event"`
	SourceSHA     string     `json:"source_sha"`
	TestedSHA     string     `json:"tested_sha"`
	BaseSHA       string     `json:"base_sha,omitempty"`
	RunID         int64      `json:"run_id"`
	RunAttempt    int64      `json:"run_attempt"`
	Jobs          []Job      `json:"jobs"`
	Artifacts     []Artifact `json:"artifacts"`
	Files         []File     `json:"files"`
}

type Envelope struct {
	Receipt       Receipt `json:"receipt"`
	ReceiptDigest string  `json:"receipt_digest"`
}

func (r Receipt) Validate() error {
	if r.SchemaVersion != SchemaVersion || !repoName.MatchString(r.Repository) || r.Workflow != "CI" {
		return fmt.Errorf("invalid evidence identity")
	}
	if r.Event != "pull_request" && r.Event != "push" {
		return fmt.Errorf("unsupported workflow event")
	}
	if !sha40.MatchString(r.SourceSHA) || !sha40.MatchString(r.TestedSHA) {
		return fmt.Errorf("invalid source/tested SHA")
	}
	if r.Event == "pull_request" {
		if !sha40.MatchString(r.BaseSHA) {
			return fmt.Errorf("pull request evidence requires base SHA")
		}
	} else if r.BaseSHA != "" {
		return fmt.Errorf("push evidence must not invent a base SHA")
	}
	if r.RunID <= 0 || r.RunAttempt <= 0 {
		return fmt.Errorf("invalid workflow run identity")
	}
	requiredJobs := map[string]bool{
		"go":                                     false,
		"offline-container-integration":          false,
		"postgres-authority-restore-drill":         false,
		"codex-app-server-0.155.0-qualification": false,
	}
	last := ""
	for _, j := range r.Jobs {
		if j.ID <= 0 || j.Conclusion != "success" || j.Name <= last {
			return fmt.Errorf("jobs must be unique sorted successful facts")
		}
		if _, ok := requiredJobs[j.Name]; !ok {
			return fmt.Errorf("unexpected retained job %q", j.Name)
		}
		requiredJobs[j.Name] = true
		last = j.Name
	}
	for name, found := range requiredJobs {
		if !found {
			return fmt.Errorf("required job %q missing", name)
		}
	}
	if len(r.Artifacts) != 2 {
		return fmt.Errorf("exactly qualification and binary artifacts required")
	}
	seenArtifact := map[string]bool{}
	last = ""
	for _, a := range r.Artifacts {
		if a.ID <= 0 || a.Size <= 0 || !canonical.ValidDigest(a.Digest) || a.Name <= last {
			return fmt.Errorf("invalid or unsorted artifact fact")
		}
		switch {
		case strings.HasPrefix(a.Name, "codex-0.155.0-qualification-"):
			seenArtifact["codex"] = true
		case strings.HasPrefix(a.Name, "engineering-binaries-"):
			seenArtifact["binaries"] = true
		default:
			return fmt.Errorf("unexpected artifact %q", a.Name)
		}
		last = a.Name
	}
	if !seenArtifact["codex"] || !seenArtifact["binaries"] {
		return fmt.Errorf("required artifacts missing")
	}
	requiredFiles := map[string]bool{
		"codex-qualifier": false,
		"control-plane":   false,
		"eng":             false,
		"sandbox-guard":   false,
		"worker":          false,
	}
	last = ""
	for _, f := range r.Files {
		if f.Path <= last || f.Size <= 0 || !canonical.ValidDigest(f.Digest) {
			return fmt.Errorf("invalid or unsorted binary fact")
		}
		if _, ok := requiredFiles[f.Path]; !ok {
			return fmt.Errorf("unexpected binary %q", f.Path)
		}
		requiredFiles[f.Path] = true
		last = f.Path
	}
	for name, found := range requiredFiles {
		if !found {
			return fmt.Errorf("required binary %q missing", name)
		}
	}
	return nil
}

func (r Receipt) Digest() (string, error) {
	if err := r.Validate(); err != nil {
		return "", err
	}
	return canonical.Digest(r)
}

func NewEnvelope(r Receipt) (Envelope, error) {
	digest, err := r.Digest()
	if err != nil {
		return Envelope{}, err
	}
	return Envelope{Receipt: r, ReceiptDigest: digest}, nil
}

func (e Envelope) Verify() error {
	digest, err := e.Receipt.Digest()
	if err != nil {
		return err
	}
	if e.ReceiptDigest != digest {
		return fmt.Errorf("receipt digest mismatch")
	}
	return nil
}

func Sort(r *Receipt) {
	sort.Slice(r.Jobs, func(i, j int) bool { return r.Jobs[i].Name < r.Jobs[j].Name })
	sort.Slice(r.Artifacts, func(i, j int) bool { return r.Artifacts[i].Name < r.Artifacts[j].Name })
	sort.Slice(r.Files, func(i, j int) bool { return r.Files[i].Path < r.Files[j].Path })
}
