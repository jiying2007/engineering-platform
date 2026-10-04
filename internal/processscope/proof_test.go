package processscope

import (
	"testing"
	"time"
)

func TestProofRejectsUnconfirmedAndForeignScope(t *testing.T) {
	now := time.Now().UTC()
	p := Proof{1, Mechanism, 123, 456, 789, now, now.Add(time.Second), true}
	if !p.Quiescent() {
		t.Fatal("fixture proof rejected")
	}
	for _, change := range []func(*Proof){
		func(p *Proof) { p.Version = 0 }, func(p *Proof) { p.Mechanism = "kill-process-group" },
		func(p *Proof) { p.NamespaceID = p.ParentNamespaceID }, func(p *Proof) { p.NamespaceID = 0 },
		func(p *Proof) { p.HostPID = 0 }, func(p *Proof) { p.InitReaped = false },
		func(p *Proof) { p.ReapedAt = time.Time{} }, func(p *Proof) { p.ReapedAt = p.StartedAt.Add(-time.Second) },
	} {
		q := p
		change(&q)
		if q.Quiescent() {
			t.Fatalf("unsafe proof: %+v", q)
		}
	}
	p.InitReaped = false
	p.ReapedAt = time.Time{}
	if p.Validate() != nil || p.Quiescent() {
		t.Fatal("unconfirmed observation changed into proof")
	}
}
