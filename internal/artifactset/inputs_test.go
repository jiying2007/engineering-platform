//go:build linux

package artifactset

import (
	"context"
	"os"
	"testing"
)

func TestVerifyProducerInputsIsReadOnlyAndStrict(t *testing.T) {
	p, path, sum := fixture(t)
	before, err := os.ReadFile(path)
	must(t, err)
	must(t, VerifyInputs(context.Background(), p))
	after, err := os.ReadFile(path)
	must(t, err)
	if string(before) != string(after) {
		t.Fatal("validation changed source plan", sum)
	}
	for _, mutate := range []func(*Plan){
		func(p *Plan) { p.Members[1].Path = p.Members[0].Path },
		func(p *Plan) { p.Members[0].Digest = p.Members[1].Digest },
		func(p *Plan) { p.Members[0].Size++ },
		func(p *Plan) { p.Members = nil },
		func(p *Plan) { p.Subject.RunID = "" },
	} {
		bad := p
		bad.Members = append([]Input{}, p.Members...)
		mutate(&bad)
		if VerifyInputs(context.Background(), bad) == nil {
			t.Fatal("invalid producer inputs accepted")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if VerifyInputs(ctx, p) == nil {
		t.Fatal("cancelled input verification accepted")
	}
}
