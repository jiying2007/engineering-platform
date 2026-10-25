package production

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"time"

	"github.com/jiying2007/engineering-platform/internal/strictjson"
)

// The event budget includes its terminating newline, matching monitor output.
const (
	ObserverMaxEventBytes   = 8 << 20
	ObserverMaxJournalBytes = 128 << 20
)

// ObserverJournalCheck describes only the supplied, contiguous JSONL prefix.
// Its digest binds these bytes; it does not authenticate their producer, prove
// that a tail was never deleted, or detect rollback between separate checks.
// Expected session and endpoint identities must be selected outside the journal.
type ObserverJournalCheck struct {
	Version             int       `json:"version"`
	Scope               string    `json:"scope"`
	SessionID           string    `json:"session_id"`
	EndpointDigest      string    `json:"endpoint_digest"`
	CheckedAt           time.Time `json:"checked_at"`
	JournalDigest       string    `json:"journal_digest"`
	JournalBytes        int64     `json:"journal_bytes"`
	RecordCount         int       `json:"record_count"`
	LastSequence        int       `json:"last_sequence"`
	LastRecordedAt      time.Time `json:"last_recorded_at"`
	ExpiresAt           time.Time `json:"expires_at"`
	State               string    `json:"state"`
	Reason              string    `json:"reason"`
	ObserverFresh       bool      `json:"observer_fresh"`
	SourceFresh         bool      `json:"source_fresh"`
	Stopped             bool      `json:"stopped"`
	CurrentClear        bool      `json:"current_clear"`
	HadAlert            bool      `json:"had_alert"`
	Ready               bool      `json:"ready"`
	ExecutionAuthorized bool      `json:"execution_authorized"`
	ProductionQualified bool      `json:"production_qualified"`
	// Retained in memory for freshness refresh after the caller's final file
	// identity checks. The summary does not duplicate a potentially large window.
	LatestEvent ObserverEvent `json:"-"`
}

// Refresh uses the already checked final event without rereading or changing
// the prefix digest. Callers can refresh after their own file identity checks.
func (c ObserverJournalCheck) Refresh(at time.Time) (ObserverJournalCheck, error) {
	if !c.CheckedAt.IsZero() && at.Before(c.CheckedAt) {
		return ObserverJournalCheck{}, fmt.Errorf("observer journal freshness clock moved backwards")
	}
	freshness, err := EvaluateObserverFreshness(c.LatestEvent, c.SessionID, c.EndpointDigest, at)
	if err != nil {
		return ObserverJournalCheck{}, err
	}
	c.CheckedAt = at.UTC()
	c.LastSequence, c.LastRecordedAt = c.LatestEvent.Sequence, c.LatestEvent.RecordedAt
	c.ExpiresAt, c.State, c.Reason = c.LatestEvent.ExpiresAt, c.LatestEvent.State, c.LatestEvent.Reason
	c.ObserverFresh, c.SourceFresh, c.Stopped = freshness.ObserverFresh, freshness.SourceFresh, freshness.Stopped
	c.CurrentClear = freshness.ObserverFresh && freshness.SourceFresh && c.LatestEvent.State == ObserverClear
	c.HadAlert = c.LatestEvent.HadAlert
	c.Ready, c.ExecutionAuthorized, c.ProductionQualified = false, false, false
	return c, nil
}

// CheckObserverJournal checks the caller Context between bounded reads and
// records. An arbitrary blocked Reader still needs a caller-owned I/O deadline.
// Sample the clock only after complete EOF and replay, so parsing time cannot
// make an expired event appear fresh. No independent Context root is created.
func CheckObserverJournal(ctx context.Context, r io.Reader, session, endpoint string, now func() time.Time) (ObserverJournalCheck, error) {
	return checkObserverJournal(ctx, r, session, endpoint, now, ObserverMaxEventBytes, ObserverMaxJournalBytes)
}

func checkObserverJournal(ctx context.Context, r io.Reader, session, endpoint string, now func() time.Time, eventBytes int, journalBytes int64) (ObserverJournalCheck, error) {
	var zero ObserverJournalCheck
	if ctx == nil || r == nil || now == nil || !observerSession.MatchString(session) || !observerDigest.MatchString(endpoint) ||
		eventBytes <= 0 || eventBytes > ObserverMaxEventBytes || journalBytes <= 0 || journalBytes > ObserverMaxJournalBytes {
		return zero, fmt.Errorf("observer journal requires expected identity, caller lifecycle and bounded input")
	}
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	// One sentinel byte detects an oversized journal. Scanner's buffer limit
	// independently bounds a single line before JSON allocation or decoding.
	limited := &io.LimitedReader{R: observerJournalReader{ctx, r}, N: journalBytes + 1}
	scanner := bufio.NewScanner(limited)
	scanner.Buffer(make([]byte, min(4096, eventBytes+1)), eventBytes+1)
	scanner.Split(func(data []byte, atEOF bool) (int, []byte, error) {
		if end := bytes.IndexByte(data, '\n'); end >= 0 {
			if end+1 > eventBytes {
				return 0, nil, fmt.Errorf("observer journal event exceeds byte limit")
			}
			return end + 1, data[:end+1], nil
		}
		if len(data) > eventBytes {
			return 0, nil, fmt.Errorf("observer journal event exceeds byte limit")
		}
		if atEOF && len(data) != 0 {
			return 0, nil, fmt.Errorf("observer journal has an incomplete final line")
		}
		return 0, nil, nil
	})
	hash := sha256.New()
	check := ObserverJournalCheck{Version: 1, Scope: "observer-journal-check", SessionID: session, EndpointDigest: endpoint}
	var observer *Observer
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return zero, err
		}
		raw := scanner.Bytes()
		if int64(len(raw)) > journalBytes-check.JournalBytes {
			return zero, fmt.Errorf("observer journal exceeds total byte limit")
		}
		check.JournalBytes += int64(len(raw))
		_, _ = hash.Write(raw)
		event, err := decodeObserverJournalEvent(raw[:len(raw)-1], eventBytes)
		if err != nil {
			return zero, fmt.Errorf("observer journal record %d: %w", check.RecordCount+1, err)
		}
		var expected ObserverEvent
		if observer == nil {
			if event.Kind != "STARTED" {
				return zero, fmt.Errorf("observer journal must begin with STARTED")
			}
			observer, err = NewObserver(event.Policy, session, endpoint, event.RecordedAt)
			if err == nil {
				expected, err = observer.Start(event.RecordedAt)
			}
		} else {
			switch event.Kind {
			case "WINDOW", "RECOVERED":
				if event.Window == nil {
					return zero, fmt.Errorf("observer journal window missing")
				}
				expected, err = observer.Window(*event.Window, event.RecordedAt)
			case "FAILED":
				// A partial first window can pin an identity without producing a
				// complete report. Replay this claimed binding; these retained
				// fields do not prove the original request or source response.
				if event.RecoveryIdentityBound {
					err = observer.BindRecovery(event.RecoveryEpoch, event.RecoveryMode)
				}
				if err == nil {
					expected, err = observer.Failure(event.Reason, event.RecordedAt)
				}
			case "STOPPED":
				expected, err = observer.Stop(event.Reason, event.RecordedAt)
			default:
				return zero, fmt.Errorf("observer journal contains an invalid event order or kind")
			}
		}
		if err != nil {
			return zero, fmt.Errorf("observer journal record %d cannot be replayed: %w", check.RecordCount+1, err)
		}
		if !reflect.DeepEqual(expected, event) {
			return zero, fmt.Errorf("observer journal record %d contradicts replay", check.RecordCount+1)
		}
		if err := ctx.Err(); err != nil {
			return zero, err
		}
		check.RecordCount++
		check.LatestEvent = event
	}
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	if err := scanner.Err(); err != nil {
		return zero, fmt.Errorf("observer journal read: %w", err)
	}
	if check.RecordCount == 0 {
		return zero, fmt.Errorf("observer journal is empty")
	}
	check.JournalDigest = "sha256:" + hex.EncodeToString(hash.Sum(nil))
	check, err := check.Refresh(now())
	if err != nil {
		return zero, err
	}
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	return check, nil
}

type observerJournalReader struct {
	ctx context.Context
	r   io.Reader
}

func (r observerJournalReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	n, err := r.r.Read(p)
	if cancelled := r.ctx.Err(); cancelled != nil {
		return n, errors.Join(err, cancelled)
	}
	return n, err
}

func decodeObserverJournalEvent(raw []byte, limit int) (ObserverEvent, error) {
	var event ObserverEvent
	if err := strictjson.ValidateForeignObjectLimit(raw, limit); err != nil {
		return event, err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&event); err != nil {
		return ObserverEvent{}, err
	}
	encoded, err := json.Marshal(event)
	if err != nil {
		return ObserverEvent{}, err
	}
	// Round-trip the typed schema at every depth. Required zero-valued members
	// must be explicit; omitempty members must use the emitter's omission rules.
	// UseNumber preserves integer identity above float64's exact range.
	var original, typed any
	for _, pair := range []struct {
		bytes []byte
		value *any
	}{{raw, &original}, {encoded, &typed}} {
		decoder := json.NewDecoder(bytes.NewReader(pair.bytes))
		decoder.UseNumber()
		if err := decoder.Decode(pair.value); err != nil {
			return ObserverEvent{}, err
		}
	}
	if !reflect.DeepEqual(original, typed) {
		return ObserverEvent{}, fmt.Errorf("observer journal differs from the complete typed event schema")
	}
	return event, nil
}
