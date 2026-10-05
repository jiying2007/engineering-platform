//go:build linux

package artifactset

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/jiying2007/engineering-platform/internal/canonical"
)

// VerifyInputs checks a producer-derived plan without writing or invoking any
// producer. It confers no authority; the caller must establish its input anchor.
// Pack repeats these checks and checks for changes while retaining the bytes.
func VerifyInputs(ctx context.Context, p Plan) error {
	if len(p.Members) == 0 || len(p.Members) > MaxMembers {
		return fmt.Errorf("bounded members required")
	}
	raw, err := json.Marshal(p)
	if err != nil || int64(len(raw)) > MaxMetadata {
		return fmt.Errorf("plan exceeds metadata bound")
	}
	m := Manifest{Version: p.Version, Coverage: Coverage, Subject: p.Subject, PlanDigest: canonical.BytesDigest(raw)}
	paths := map[string]bool{}
	for _, in := range p.Members {
		if paths[in.Path] {
			return fmt.Errorf("duplicate input path")
		}
		paths[in.Path] = true
		m.Members = append(m.Members, in.Entry)
	}
	if err := m.Validate(); err != nil {
		return err
	}
	for _, in := range p.Members {
		if err := ctx.Err(); err != nil {
			return err
		}
		f, err := openInput(in.Path, MaxFile)
		if err != nil {
			return err
		}
		err = copyExact(ctx, f.file, io.Discard, in.Size, in.Digest)
		if err == nil {
			err = f.check()
		}
		f.Close()
		if err != nil {
			return err
		}
	}
	return nil
}
