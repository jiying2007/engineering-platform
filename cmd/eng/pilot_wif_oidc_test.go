package main

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func writeOIDCFixture(t *testing.T, root string, claims map[string]any) string {
	t.Helper()
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","typ":"JWT"}`))
	payload, err := json.Marshal(claims)
	if err != nil {
		t.Fatal(err)
	}
	token := header + "." + base64.RawURLEncoding.EncodeToString(payload) + ".fixture"
	path := filepath.Join(root, "oidc.jwt")
	if err := os.WriteFile(path, []byte(token), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func runOIDCVerifier(t *testing.T, tokenFile, repositoryID string) ([]byte, error) {
	t.Helper()
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 unavailable")
	}
	script, err := filepath.Abs(filepath.Join("..", "..", "examples", "pilots", "wif", "verify-github-oidc.py"))
	if err != nil {
		t.Fatal(err)
	}
	return exec.Command(
		python, script,
		"--token-file", tokenFile,
		"--audience", "aud-pilot",
		"--repository", "jiying2007/engineering-platform",
		"--repository-id", repositoryID,
		"--repository-owner-id", "33591504",
		"--ref", "refs/heads/main",
		"--workflow-ref", "jiying2007/engineering-platform/.github/workflows/retained-pilot-engineer.yml@refs/heads/main",
		"--max-lifetime-seconds", "600",
		"--min-remaining-seconds", "120",
	).CombinedOutput()
}

func githubOIDCClaims(now time.Time) map[string]any {
	return map[string]any{
		"iss":                 "https://token.actions.githubusercontent.com",
		"aud":                 "aud-pilot",
		"sub":                 "repo:jiying2007@33591504/engineering-platform@1383377268:ref:refs/heads/main",
		"repository":          "jiying2007/engineering-platform",
		"repository_id":       "1383377268",
		"repository_owner_id": "33591504",
		"ref":                 "refs/heads/main",
		"workflow_ref":        "jiying2007/engineering-platform/.github/workflows/retained-pilot-engineer.yml@refs/heads/main",
		"jti":                 "fixture-jti",
		"iat":                 now.Unix(),
		"nbf":                 now.Add(-time.Minute).Unix(),
		"exp":                 now.Add(5 * time.Minute).Unix(),
	}
}

func TestGitHubOIDCVerifierAcceptsImmutableSubjectFormat(t *testing.T) {
	now := time.Now()
	token := writeOIDCFixture(t, t.TempDir(), githubOIDCClaims(now))
	if out, err := runOIDCVerifier(t, token, "1383377268"); err != nil {
		t.Fatalf("immutable GitHub subject rejected: %v: %s", err, out)
	}
}

func TestGitHubOIDCVerifierRejectsIdentityAndTimingDrift(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name         string
		mutate       func(map[string]any)
		repositoryID string
	}{
		{
			name: "repository-id",
			mutate: func(map[string]any) {},
			repositoryID: "999",
		},
		{
			name: "missing-jti",
			mutate: func(c map[string]any) { delete(c, "jti") },
			repositoryID: "1383377268",
		},
		{
			name: "provider-lifetime",
			mutate: func(c map[string]any) {
				c["exp"] = now.Add(601 * time.Second).Unix()
			},
			repositoryID: "1383377268",
		},
		{
			name: "remaining-lifetime",
			mutate: func(c map[string]any) {
				c["iat"] = now.Add(-4 * time.Minute).Unix()
				c["exp"] = now.Add(60 * time.Second).Unix()
			},
			repositoryID: "1383377268",
		},
	}
	for index, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			claims := githubOIDCClaims(now)
			tc.mutate(claims)
			claims["jti"] = func() any {
				if tc.name == "missing-jti" {
					return nil
				}
				return "fixture-jti-" + strconv.Itoa(index)
			}()
			if tc.name == "missing-jti" {
				delete(claims, "jti")
			}
			token := writeOIDCFixture(t, t.TempDir(), claims)
			if out, err := runOIDCVerifier(t, token, tc.repositoryID); err == nil {
				t.Fatalf("unsafe OIDC assertion accepted: %s", out)
			}
		})
	}
}
