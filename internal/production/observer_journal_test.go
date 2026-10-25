package production

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
	"testing/iotest"
	"time"
)

func observerJournalFixture(t *testing.T) ([]ObserverEvent, time.Time) {
	t.Helper()
	p := observerTestPolicy()
	start := time.Date(2026, 10, 25, 0, 0, 0, 0, time.UTC)
	o, err := NewObserver(p, testObserverSession, testObserverEndpoint, start)
	if err != nil {
		t.Fatal(err)
	}
	var events []ObserverEvent
	add := func(event ObserverEvent, err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		events = append(events, event)
	}
	add(o.Start(start))
	// A valid first source response can pin recovery identity before another
	// sample fails. The FAILED record retains the claim, not the source bytes.
	if err := o.BindRecovery(2, "NORMAL"); err != nil {
		t.Fatal(err)
	}
	add(o.Failure("SOURCE_UNAVAILABLE", start.Add(time.Second)))
	add(o.Window(observerTestWindow(t, p.Progress, start.Add(3*time.Second), 2, false), start.Add(5*time.Second)))
	add(o.Window(observerTestWindow(t, p.Progress, start.Add(6*time.Second), 2, true), start.Add(8*time.Second)))
	add(o.Window(observerTestWindow(t, p.Progress, start.Add(9*time.Second), 2, false), start.Add(11*time.Second)))
	add(o.Stop("SCHEDULE_COMPLETE", start.Add(11*time.Second)))
	return events, start
}

func observerJournalBytes(t *testing.T, events []ObserverEvent) []byte {
	t.Helper()
	var out bytes.Buffer
	for _, event := range events {
		if err := json.NewEncoder(&out).Encode(event); err != nil {
			t.Fatal(err)
		}
	}
	return out.Bytes()
}

func observerJournalAt(at time.Time) func() time.Time { return func() time.Time { return at } }

func TestObserverJournalReplaysPrefixesAndRefreshesFreshness(t *testing.T) {
	events, start := observerJournalFixture(t)
	for _, tc := range []struct {
		name                    string
		count, seconds          int
		observer, source, clear bool
		alert, stopped          bool
	}{
		{"waiting", 1, 0, true, false, false, false, false},
		{"first-partial-window-bound", 2, 1, true, false, false, true, false},
		{"recovered-with-history", 3, 8, true, true, true, true, false},
		{"diagnostic-alert", 4, 8, true, true, false, true, false},
		{"recovered-again", 5, 11, true, true, true, true, false},
		{"stopped", 6, 11, false, false, false, true, true},
		{"expired-at-boundary", 5, 15, false, false, false, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := observerJournalBytes(t, events[:tc.count])
			at := start.Add(time.Duration(tc.seconds) * time.Second)
			check, err := CheckObserverJournal(context.Background(), iotest.OneByteReader(bytes.NewReader(raw)), testObserverSession, testObserverEndpoint, observerJournalAt(at))
			if err != nil {
				t.Fatal(err)
			}
			hash := sha256.Sum256(raw)
			last := events[tc.count-1]
			if check.Version != 1 || check.Scope != "observer-journal-check" || check.SessionID != testObserverSession || check.EndpointDigest != testObserverEndpoint ||
				check.JournalDigest != "sha256:"+hex.EncodeToString(hash[:]) || check.JournalBytes != int64(len(raw)) || check.RecordCount != tc.count || check.LastSequence != tc.count ||
				!check.CheckedAt.Equal(at) || !check.LastRecordedAt.Equal(last.RecordedAt) || !check.ExpiresAt.Equal(last.ExpiresAt) || check.State != last.State || check.Reason != last.Reason ||
				check.ObserverFresh != tc.observer || check.SourceFresh != tc.source || check.CurrentClear != tc.clear || check.HadAlert != tc.alert || check.Stopped != tc.stopped ||
				check.Ready || check.ExecutionAuthorized || check.ProductionQualified || !reflect.DeepEqual(check.LatestEvent, last) {
				t.Fatalf("incorrect prefix check: %+v", check)
			}
		})
	}
	raw := observerJournalBytes(t, events[:3])
	check, err := CheckObserverJournal(context.Background(), bytes.NewReader(raw), testObserverSession, testObserverEndpoint, observerJournalAt(start.Add(5*time.Second)))
	if err != nil || !check.CurrentClear {
		t.Fatal(check, err)
	}
	refreshed, err := check.Refresh(start.Add(9 * time.Second))
	if err != nil || refreshed.CurrentClear || refreshed.ObserverFresh || refreshed.SourceFresh || !refreshed.HadAlert ||
		refreshed.JournalDigest != check.JournalDigest || refreshed.JournalBytes != check.JournalBytes || refreshed.RecordCount != check.RecordCount {
		t.Fatal(refreshed, err)
	}
	if bad, err := check.Refresh(start); err == nil || bad != (ObserverJournalCheck{}) {
		t.Fatal("refresh before final record accepted")
	}
	if bad, err := refreshed.Refresh(start.Add(8 * time.Second)); err == nil || bad != (ObserverJournalCheck{}) {
		t.Fatal("expired check became current clear through a backwards refresh")
	}
}

func TestObserverJournalSourceFreshnessAndEOFTiming(t *testing.T) {
	p := observerTestPolicy()
	start := time.Date(2026, 10, 25, 0, 0, 0, 0, time.UTC)
	o, _ := NewObserver(p, testObserverSession, testObserverEndpoint, start)
	first, _ := o.Start(start)
	last, err := o.Window(observerTestWindow(t, p.Progress, start, 2, false), start.Add(3*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	raw := observerJournalBytes(t, []ObserverEvent{first, last})
	check, err := CheckObserverJournal(context.Background(), bytes.NewReader(raw), testObserverSession, testObserverEndpoint, observerJournalAt(start.Add(6*time.Second)))
	if err != nil || !check.ObserverFresh || check.SourceFresh || check.CurrentClear || check.HadAlert {
		t.Fatalf("source expiry gained observer freshness: %+v %v", check, err)
	}
	clock := last.RecordedAt
	source := bytes.NewReader(raw)
	seenEOF, clockCalls := false, 0
	reader := observerJournalReadFunc(func(p []byte) (int, error) {
		n, err := source.Read(p)
		if err == io.EOF {
			seenEOF = true
			clock = last.ExpiresAt
		}
		return n, err
	})
	check, err = CheckObserverJournal(context.Background(), reader, testObserverSession, testObserverEndpoint, func() time.Time {
		clockCalls++
		if !seenEOF {
			t.Fatal("freshness clock sampled before complete EOF")
		}
		return clock
	})
	if err != nil || clockCalls != 1 || check.ObserverFresh || check.SourceFresh || check.CurrentClear || !check.CheckedAt.Equal(last.ExpiresAt) {
		t.Fatalf("read time was excluded from freshness: %+v %v calls=%d", check, err, clockCalls)
	}
}

func TestObserverJournalWireAndSchemaAreComplete(t *testing.T) {
	events, start := observerJournalFixture(t)
	valid := observerJournalBytes(t, events[:3])
	for _, tc := range []struct {
		name string
		raw  []byte
	}{
		{"empty", nil},
		{"blank", []byte("\n")},
		{"blank-tail", append(append([]byte{}, valid...), []byte(" \t\n")...)},
		{"partial-final-record", valid[:len(valid)-1]},
		{"partial-tail", append(append([]byte{}, valid...), []byte(`{"version":`)...)},
		{"null-object", []byte("null\n")},
		{"array-object", []byte("[]\n")},
		{"duplicate-top-member", bytes.Replace(valid, []byte(`"version":1`), []byte(`"version":1,"version":1`), 1)},
		{"duplicate-nested-member", bytes.Replace(valid, []byte(`"interval_seconds":1`), []byte(`"interval_seconds":1,"interval_seconds":1`), 1)},
		{"invalid-utf8", append([]byte{'{', '"', 0xff, '"', ':', '0', '}'}, '\n')},
		{"multiple-objects-in-line", bytes.Replace(valid, []byte("}\n{"), []byte("}{"), 1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			check, err := CheckObserverJournal(context.Background(), bytes.NewReader(tc.raw), testObserverSession, testObserverEndpoint, observerJournalAt(start.Add(8*time.Second)))
			if err == nil || !reflect.DeepEqual(check, ObserverJournalCheck{}) {
				t.Fatalf("invalid wire accepted: %+v %v", check, err)
			}
		})
	}
	type object = map[string]any
	window := func(m object) object { return m["window"].(object) }
	status := func(m object) object { return window(m)["observations"].([]any)[0].(object)["status"].(object) }
	snapshot := func(m object) object { return status(m)["snapshot"].(object) }
	for _, tc := range []struct {
		name   string
		index  int
		mutate func(object)
	}{
		{"missing-false-field", 2, func(m object) { delete(m, "ready") }},
		{"null-false-field", 2, func(m object) { m["ready"] = nil }},
		{"missing-zero-field", 0, func(m object) { delete(m, "failed_windows") }},
		{"unknown-field", 2, func(m object) { m["unknown"] = false }},
		{"case-alias", 2, func(m object) { delete(m, "ready"); m["Ready"] = false }},
		{"wrong-scalar-type", 2, func(m object) { m["sequence"] = "3" }},
		{"optional-window-null", 0, func(m object) { m["window"] = nil }},
		{"optional-success-null", 0, func(m object) { m["last_success_at"] = nil }},
		{"optional-mode-empty", 0, func(m object) { m["recovery_mode"] = "" }},
		{"missing-policy-field", 2, func(m object) { delete(m["policy"].(object)["progress"].(object), "max_pending_age_seconds") }},
		{"null-policy-field", 2, func(m object) { m["policy"].(object)["progress"].(object)["interval_seconds"] = nil }},
		{"unknown-nested-field", 2, func(m object) { window(m)["unknown"] = false }},
		{"missing-window-false", 2, func(m object) { delete(window(m), "capacity_observed") }},
		{"missing-queue-zero", 2, func(m object) { delete(window(m)["worker_queue"].(object), "peak_pending_at_samples") }},
		{"missing-status-false", 2, func(m object) { delete(status(m), "ready") }},
		{"null-status-fact", 2, func(m object) { status(m)["authority_clear"] = nil }},
		{"optional-status-reasons-empty", 2, func(m object) { status(m)["reasons"] = []any{} }},
		{"null-snapshot-fact", 2, func(m object) { snapshot(m)["pending_outbox"] = nil }},
		{"missing-nullable-snapshot-field", 2, func(m object) { delete(snapshot(m), "oldest_pending_worker_at") }},
		{"null-worker-polls", 2, func(m object) { snapshot(m)["worker_polls"] = nil }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lines := bytes.Split(bytes.TrimSuffix(valid, []byte("\n")), []byte("\n"))
			var value object
			decoder := json.NewDecoder(bytes.NewReader(lines[tc.index]))
			decoder.UseNumber()
			if err := decoder.Decode(&value); err != nil {
				t.Fatal(err)
			}
			tc.mutate(value)
			changed, err := json.Marshal(value)
			if err != nil {
				t.Fatal(err)
			}
			lines[tc.index] = changed
			raw := append(bytes.Join(lines, []byte("\n")), '\n')
			check, err := CheckObserverJournal(context.Background(), bytes.NewReader(raw), testObserverSession, testObserverEndpoint, observerJournalAt(start.Add(8*time.Second)))
			if err == nil || !reflect.DeepEqual(check, ObserverJournalCheck{}) {
				t.Fatalf("incomplete/shifted schema accepted: %+v %v", check, err)
			}
		})
	}
}

func TestObserverJournalRejectsHistoryIdentityAndStateChanges(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func([]ObserverEvent, time.Time) []ObserverEvent
	}{
		{"tail-only", func(e []ObserverEvent, _ time.Time) []ObserverEvent { return e[1:] }},
		{"missing-middle", func(e []ObserverEvent, _ time.Time) []ObserverEvent { return append(e[:1], e[2:]...) }},
		{"duplicate-start", func(e []ObserverEvent, _ time.Time) []ObserverEvent { return []ObserverEvent{e[0], e[0]} }},
		{"sequence-gap", func(e []ObserverEvent, _ time.Time) []ObserverEvent { e[2].Sequence++; return e }},
		{"session-drift", func(e []ObserverEvent, _ time.Time) []ObserverEvent {
			e[2].SessionID = strings.Repeat("b", 32)
			return e
		}},
		{"endpoint-drift", func(e []ObserverEvent, _ time.Time) []ObserverEvent {
			e[2].EndpointDigest = "sha256:" + strings.Repeat("b", 64)
			return e
		}},
		{"policy-drift", func(e []ObserverEvent, _ time.Time) []ObserverEvent { e[2].Policy.StaleAfterSeconds++; return e }},
		{"epoch-drift", func(e []ObserverEvent, _ time.Time) []ObserverEvent { e[2].RecoveryEpoch++; return e }},
		{"source-epoch-drift", func(e []ObserverEvent, start time.Time) []ObserverEvent {
			r := observerTestWindow(t, e[2].Policy.Progress, start.Add(3*time.Second), 3, false)
			e[2].Window, e[2].RecoveryEpoch = &r, 3
			return e
		}},
		{"first-binding-mode-drift", func(e []ObserverEvent, _ time.Time) []ObserverEvent {
			e[1].RecoveryMode = "RECOVERY_RECONCILIATION"
			return e
		}},
		{"removed-binding", func(e []ObserverEvent, _ time.Time) []ObserverEvent {
			e[2].RecoveryIdentityBound = false
			e[2].RecoveryEpoch = 0
			e[2].RecoveryMode = ""
			return e
		}},
		{"backwards-time", func(e []ObserverEvent, start time.Time) []ObserverEvent { e[2].RecordedAt = start; return e }},
		{"changed-expiry", func(e []ObserverEvent, _ time.Time) []ObserverEvent {
			e[2].ExpiresAt = e[2].ExpiresAt.Add(time.Second)
			return e
		}},
		{"changed-state-since", func(e []ObserverEvent, _ time.Time) []ObserverEvent {
			e[2].StateSince = e[2].StateSince.Add(-time.Second)
			return e
		}},
		{"forgotten-alert", func(e []ObserverEvent, _ time.Time) []ObserverEvent { e[2].HadAlert = false; return e }},
		{"changed-counter", func(e []ObserverEvent, _ time.Time) []ObserverEvent { e[2].FailedWindows = 0; return e }},
		{"changed-recovery-kind", func(e []ObserverEvent, _ time.Time) []ObserverEvent { e[2].Kind = "WINDOW"; return e }},
		{"unknown-kind", func(e []ObserverEvent, _ time.Time) []ObserverEvent { e[2].Kind = "HEALTHY"; return e }},
		{"invented-ready", func(e []ObserverEvent, _ time.Time) []ObserverEvent { e[2].Ready = true; return e }},
		{"forged-window", func(e []ObserverEvent, _ time.Time) []ObserverEvent { e[2].Window.Complete = false; return e }},
		{"after-stop", func(e []ObserverEvent, _ time.Time) []ObserverEvent { return append(e, e[4]) }},
		{"early-schedule-stop", func(e []ObserverEvent, _ time.Time) []ObserverEvent {
			e[5].Sequence = 2
			return []ObserverEvent{e[0], e[5]}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			events, start := observerJournalFixture(t)
			raw := observerJournalBytes(t, tc.mutate(events, start))
			check, err := CheckObserverJournal(context.Background(), bytes.NewReader(raw), testObserverSession, testObserverEndpoint, observerJournalAt(start.Add(12*time.Second)))
			if err == nil || !reflect.DeepEqual(check, ObserverJournalCheck{}) {
				t.Fatalf("changed history accepted: %+v %v", check, err)
			}
		})
	}
}

type observerJournalReadFunc func([]byte) (int, error)

func (f observerJournalReadFunc) Read(p []byte) (int, error) { return f(p) }

func TestObserverJournalBoundsReadersErrorsAndCancellation(t *testing.T) {
	events, start := observerJournalFixture(t)
	raw := observerJournalBytes(t, events[:2])
	lines := bytes.Split(bytes.TrimSuffix(raw, []byte("\n")), []byte("\n"))
	maxLine := max(len(lines[0]), len(lines[1])) + 1
	check, err := checkObserverJournal(context.Background(), bytes.NewReader(raw), testObserverSession, testObserverEndpoint, observerJournalAt(start.Add(time.Second)), maxLine, int64(len(raw)))
	if err != nil || check.JournalBytes != int64(len(raw)) {
		t.Fatal("exact inclusive byte budgets rejected", check, err)
	}
	for _, limits := range [][2]int{{maxLine - 1, len(raw)}, {maxLine, len(raw) - 1}, {0, len(raw)}, {maxLine, 0}} {
		check, err := checkObserverJournal(context.Background(), bytes.NewReader(raw), testObserverSession, testObserverEndpoint, observerJournalAt(start.Add(time.Second)), limits[0], int64(limits[1]))
		if err == nil || !reflect.DeepEqual(check, ObserverJournalCheck{}) {
			t.Fatalf("byte limit not enforced: %v %+v %v", limits, check, err)
		}
	}
	reads := 0
	unending := observerJournalReadFunc(func(p []byte) (int, error) {
		for i := range p {
			p[i] = ' '
		}
		reads += len(p)
		return len(p), nil
	})
	if _, err := checkObserverJournal(context.Background(), unending, testObserverSession, testObserverEndpoint, observerJournalAt(start), 31, 63); err == nil || reads > 32 {
		t.Fatalf("line size not bounded before parse: err=%v read=%d", err, reads)
	}
	reads = 0
	source := bytes.NewReader(raw)
	counted := observerJournalReadFunc(func(p []byte) (int, error) {
		n, err := source.Read(p)
		reads += n
		return n, err
	})
	if _, err := checkObserverJournal(context.Background(), counted, testObserverSession, testObserverEndpoint, observerJournalAt(start), maxLine, 31); err == nil || reads > 32 {
		t.Fatalf("total size not bounded before parse: err=%v read=%d", err, reads)
	}
	sentinel := errors.New("retained reader failure")
	for _, withBytes := range []bool{false, true} {
		source := bytes.NewReader(raw)
		reader := observerJournalReadFunc(func(p []byte) (int, error) {
			n, err := source.Read(p)
			if (withBytes && n > 0) || err == io.EOF {
				err = sentinel
			}
			return n, err
		})
		check, err := CheckObserverJournal(context.Background(), reader, testObserverSession, testObserverEndpoint, observerJournalAt(start.Add(time.Second)))
		if !errors.Is(err, sentinel) || !reflect.DeepEqual(check, ObserverJournalCheck{}) {
			t.Fatal("reader failure became a valid prefix", check, err)
		}
	}
	for _, when := range []string{"before", "during", "eof", "clock"} {
		t.Run(when, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if when == "before" {
				cancel()
			}
			source := bytes.NewReader(raw)
			reader := observerJournalReadFunc(func(p []byte) (int, error) {
				if when == "before" {
					t.Fatal("cancelled caller reached reader")
				}
				n, err := source.Read(p)
				if when == "during" || (when == "eof" && err == io.EOF) {
					cancel()
				}
				return n, err
			})
			check, err := CheckObserverJournal(ctx, reader, testObserverSession, testObserverEndpoint, func() time.Time {
				if when == "clock" {
					cancel()
				}
				return start.Add(time.Second)
			})
			if !errors.Is(err, context.Canceled) || !reflect.DeepEqual(check, ObserverJournalCheck{}) {
				t.Fatal("cancelled caller obtained a check", check, err)
			}
		})
	}
}

func TestObserverJournalFormattingAndExplicitIdentity(t *testing.T) {
	events, start := observerJournalFixture(t)
	var formatted bytes.Buffer
	for _, event := range events[:3] {
		raw := observerJournalBytes(t, []ObserverEvent{event})
		var object map[string]json.RawMessage
		if err := json.Unmarshal(raw, &object); err != nil {
			t.Fatal(err)
		}
		// Reordered keys, whitespace and CRLF are data-format variations. The
		// raw digest must still bind precisely the supplied bytes.
		reordered, err := json.Marshal(object)
		if err != nil {
			t.Fatal(err)
		}
		formatted.WriteString(" \t")
		formatted.Write(reordered)
		formatted.WriteString(" \r\n")
	}
	check, err := CheckObserverJournal(context.Background(), bytes.NewReader(formatted.Bytes()), testObserverSession, testObserverEndpoint, observerJournalAt(start.Add(5*time.Second)))
	hash := sha256.Sum256(formatted.Bytes())
	if err != nil || !check.CurrentClear || check.JournalDigest != "sha256:"+hex.EncodeToString(hash[:]) {
		t.Fatal(check, err)
	}
	for _, identity := range [][2]string{{"", testObserverEndpoint}, {testObserverSession, ""}, {strings.Repeat("b", 32), testObserverEndpoint}, {testObserverSession, "sha256:" + strings.Repeat("b", 64)}} {
		if _, err := CheckObserverJournal(context.Background(), bytes.NewReader(formatted.Bytes()), identity[0], identity[1], observerJournalAt(start.Add(5*time.Second))); err == nil {
			t.Fatal("unselected journal identity accepted", identity)
		}
	}
	o, _ := NewObserver(observerTestPolicy(), testObserverSession, testObserverEndpoint, start)
	first, _ := o.Start(start)
	if err := o.BindRecovery(1<<62, "NORMAL"); err != nil {
		t.Fatal(err)
	}
	last, err := o.Failure("SOURCE_UNAVAILABLE", start.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	check, err = CheckObserverJournal(context.Background(), bytes.NewReader(observerJournalBytes(t, []ObserverEvent{first, last})), testObserverSession, testObserverEndpoint, observerJournalAt(start.Add(time.Second)))
	if err != nil || check.LatestEvent.RecoveryEpoch != 1<<62 || check.CurrentClear || check.Ready || check.ExecutionAuthorized || check.ProductionQualified {
		t.Fatal("large exact claimed identity was lost or granted authority", check, err)
	}
}
