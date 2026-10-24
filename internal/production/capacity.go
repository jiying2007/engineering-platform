package production

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/jiying2007/engineering-platform/internal/canonical"
	"github.com/jiying2007/engineering-platform/internal/strictjson"
	"github.com/jiying2007/engineering-platform/internal/workerqueue"
)

const (
	CapacityVersion = 1
	CapacityScope   = "authenticated-core-instance-capacity"
)

var capacitySubject = regexp.MustCompile(`^urn:engineering-platform:[A-Za-z0-9][A-Za-z0-9._:-]{0,191}$`)
var capacityCommit = regexp.MustCompile(`^[0-9a-f]{40}$`)
var capacityInstance = regexp.MustCompile(`^[0-9a-f]{32}$`)

// CapacitySource identifies one running Core, not an entire deployment. The
// expected instance must come from the selected host's startup record, never
// from an otherwise untrusted first response. A copied certificate/policy or a
// restarted process therefore cannot silently replace the expected source.
type CapacitySource struct {
	ServerCertificateDigest string `json:"server_certificate_digest"`
	AccessPolicyDigest      string `json:"access_policy_digest"`
	SourceCommit            string `json:"source_commit"`
	InstanceID              string `json:"instance_id"`
}

func (s CapacitySource) Validate() error {
	if !canonical.ValidDigest(s.ServerCertificateDigest) || !canonical.ValidDigest(s.AccessPolicyDigest) ||
		!capacityCommit.MatchString(s.SourceCommit) || !capacityInstance.MatchString(s.InstanceID) {
		return fmt.Errorf("explicit certificate, loaded policy, clean source and Core instance identities required")
	}
	return nil
}

// WorkerCapacityGrant is copied from the Core's already loaded immutable access
// policy. It is not an operator-submitted capacity or health assertion.
type WorkerCapacityGrant struct {
	Subject  string   `json:"subject"`
	Profiles []string `json:"profiles"`
	Lane     string   `json:"lane"`
}

func CapacityProfiles(grants []WorkerCapacityGrant) ([]string, error) {
	if grants == nil || len(grants) > 256 {
		return nil, fmt.Errorf("bounded current worker grants required")
	}
	profiles := map[string]bool{}
	previous := ""
	encodedGrantBytes := 0
	for _, grant := range grants {
		if !capacitySubject.MatchString(grant.Subject) || grant.Subject <= previous ||
			(grant.Lane != "ADMISSION" && grant.Lane != "PREPARATION") || len(grant.Profiles) == 0 || len(grant.Profiles) > 64 {
			return nil, fmt.Errorf("invalid current worker grant")
		}
		previous = grant.Subject
		encodedGrantBytes += len(grant.Subject) + 128
		priorProfile := ""
		for _, profile := range grant.Profiles {
			if !workerqueue.ValidProfile(profile) || profile <= priorProfile {
				return nil, fmt.Errorf("worker grant profiles must be canonical and uniquely sorted")
			}
			priorProfile = profile
			profiles[profile] = true
			encodedGrantBytes += len(profile) + 4
		}
	}
	// This diagnostic has a bounded response. A wider valid access policy may
	// continue serving other Core APIs while this observation is unavailable.
	if len(profiles) > 256 || encodedGrantBytes > 256<<10 {
		return nil, fmt.Errorf("capacity observation exceeds bounded profile population")
	}
	result := make([]string, 0, len(profiles))
	for profile := range profiles {
		result = append(result, profile)
	}
	slices.Sort(result)
	return result, nil
}

type WorkerCapacityFact struct {
	Grant           WorkerCapacityGrant `json:"grant"`
	Recorded        bool                `json:"recorded"`
	Protocol        string              `json:"protocol"`
	Status          string              `json:"status"`
	LastPollProfile string              `json:"last_poll_profile"`
	LastPollAt      *time.Time          `json:"last_poll_at"`
	ActiveLeases    int64               `json:"active_leases"`
	ExpiredLeases   int64               `json:"expired_leases"`
}

type QueueCapacityFact struct {
	Profile         string     `json:"profile"`
	Pending         int64      `json:"pending"`
	ActiveLeases    int64      `json:"active_leases"`
	ExpiredLeases   int64      `json:"expired_leases"`
	OldestPendingAt *time.Time `json:"oldest_pending_at"`
}

// PoolCapacityFact describes only this Core process's pgx connection pool.
// Its separately recorded time is after the read-only SQL transaction releases
// its connection. It is neither PostgreSQL server capacity nor a shared-DB SLO.
type PoolCapacityFact struct {
	CapturedAt   time.Time `json:"captured_at"`
	Maximum      int64     `json:"maximum"`
	Total        int64     `json:"total"`
	Acquired     int64     `json:"acquired"`
	Idle         int64     `json:"idle"`
	Constructing int64     `json:"constructing"`
}

type CapacityFacts struct {
	Authority     OperationalStatus    `json:"authority"`
	CapturedAt    time.Time            `json:"captured_at"`
	RecoveryEpoch uint64               `json:"recovery_epoch"`
	RecoveryMode  string               `json:"recovery_mode"`
	Workers       []WorkerCapacityFact `json:"workers"`
	Queues        []QueueCapacityFact  `json:"queues"`
	Pool          PoolCapacityFact     `json:"pool"`
}

func (f CapacityFacts) Validate(grants []WorkerCapacityGrant) error {
	profiles, err := CapacityProfiles(grants)
	if err != nil {
		return err
	}
	if f.CapturedAt.IsZero() || f.RecoveryEpoch > 1<<62 ||
		(f.RecoveryMode != "NORMAL" && f.RecoveryMode != "RECOVERY_RECONCILIATION") ||
		f.Workers == nil || f.Queues == nil || len(f.Workers) != len(grants) || len(f.Queues) != len(profiles) {
		return fmt.Errorf("complete capacity authority observations required")
	}
	if f.Authority.ValidateAt(f.CapturedAt) != nil || !f.CapturedAt.Equal(f.Authority.Snapshot.CapturedAt) ||
		f.RecoveryEpoch != f.Authority.Snapshot.RecoveryEpoch || f.RecoveryMode != f.Authority.Snapshot.RecoveryMode {
		return fmt.Errorf("capacity facts differ from the original database authority snapshot")
	}
	remainingWorkerActive, remainingWorkerExpired := f.Authority.Snapshot.ActiveWorkerLeases, f.Authority.Snapshot.ExpiredWorkerLeases
	pollGroups := make(map[string]WorkerPollObservation, len(f.Authority.Snapshot.WorkerPolls))
	for _, poll := range f.Authority.Snapshot.WorkerPolls {
		pollGroups[poll.WorkerProfile] = poll
	}
	for i, worker := range f.Workers {
		if !reflect.DeepEqual(worker.Grant, grants[i]) || worker.ActiveLeases < 0 || worker.ActiveLeases > 1 || worker.ExpiredLeases < 0 {
			return fmt.Errorf("worker reservations contradict current grants or the enforced single-live-lease bound")
		}
		if worker.ActiveLeases > remainingWorkerActive || worker.ExpiredLeases > remainingWorkerExpired {
			return fmt.Errorf("selected worker reservations exceed original authority totals")
		}
		remainingWorkerActive -= worker.ActiveLeases
		remainingWorkerExpired -= worker.ExpiredLeases
		if worker.Recorded {
			if worker.Protocol == "" || len(worker.Protocol) > 128 || strings.ContainsAny(worker.Protocol, " \t\r\n\x00") ||
				worker.Status == "" || len(worker.Status) > 64 || strings.ContainsAny(worker.Status, " \t\r\n\x00") ||
				!workerqueue.ValidProfile(worker.LastPollProfile) || worker.LastPollAt == nil || worker.LastPollAt.IsZero() || worker.LastPollAt.After(f.CapturedAt) {
				return fmt.Errorf("invalid recorded worker registration")
			}
			// The original SQL groups every ONLINE registration with a string
			// profile, including old protocols and currently ungranted identities.
			// Selected rows must be a consistent subset of that retained history.
			if worker.Status == "ONLINE" {
				group, exists := pollGroups[worker.LastPollProfile]
				if !exists || group.KnownIdentities <= 0 || worker.LastPollAt.After(group.LatestPollAt) {
					return fmt.Errorf("selected registration contradicts original worker poll history")
				}
				group.KnownIdentities--
				pollGroups[worker.LastPollProfile] = group
			}
		} else if worker.Protocol != "" || worker.Status != "" || worker.LastPollProfile != "" || worker.LastPollAt != nil || worker.ActiveLeases != 0 || worker.ExpiredLeases != 0 {
			return fmt.Errorf("missing worker registration contradicts retained worker facts")
		}
	}
	remainingPending := f.Authority.Snapshot.PendingWorkerIntents
	remainingQueueActive, remainingQueueExpired := f.Authority.Snapshot.ActiveWorkerLeases, f.Authority.Snapshot.ExpiredWorkerLeases
	for i, queue := range f.Queues {
		if queue.Profile != profiles[i] || queue.Pending < 0 || queue.ActiveLeases < 0 || queue.ExpiredLeases < 0 ||
			(queue.Pending > 0) != (queue.OldestPendingAt != nil) ||
			(queue.OldestPendingAt != nil && (queue.OldestPendingAt.IsZero() || queue.OldestPendingAt.After(f.CapturedAt))) {
			return fmt.Errorf("invalid current-profile queue observation")
		}
		if queue.Pending > remainingPending || queue.ActiveLeases > remainingQueueActive || queue.ExpiredLeases > remainingQueueExpired ||
			(queue.OldestPendingAt != nil && (f.Authority.Snapshot.OldestPendingWorkerAt == nil || queue.OldestPendingAt.Before(*f.Authority.Snapshot.OldestPendingWorkerAt))) {
			return fmt.Errorf("selected profile queues contradict original authority totals or oldest time")
		}
		remainingPending -= queue.Pending
		remainingQueueActive -= queue.ActiveLeases
		remainingQueueExpired -= queue.ExpiredLeases
	}
	p := f.Pool
	if p.CapturedAt.IsZero() || p.Maximum <= 0 || p.Maximum > math.MaxInt32 || p.Total < 0 || p.Total > p.Maximum ||
		p.Acquired < 0 || p.Acquired > p.Maximum || p.Idle < 0 || p.Idle > p.Maximum || p.Constructing < 0 || p.Constructing > p.Maximum ||
		p.Acquired+p.Idle+p.Constructing != p.Total {
		return fmt.Errorf("contradictory local connection-pool observation")
	}
	return nil
}

type CapacityResponse struct {
	Version     int            `json:"version"`
	Scope       string         `json:"scope"`
	Source      CapacitySource `json:"source"`
	GeneratedAt time.Time      `json:"generated_at"`
	Facts       CapacityFacts  `json:"facts"`
}

func (r CapacityResponse) ValidateAt(now time.Time) error {
	if r.Version != CapacityVersion || r.Scope != CapacityScope || r.Source.Validate() != nil {
		return fmt.Errorf("identified Core capacity response required")
	}
	grants := make([]WorkerCapacityGrant, len(r.Facts.Workers))
	for i := range r.Facts.Workers {
		grants[i] = r.Facts.Workers[i].Grant
	}
	if err := r.Facts.Validate(grants); err != nil {
		return err
	}
	// These are finite transport/readback limits, not worker heartbeats or SLOs.
	for _, at := range []time.Time{r.GeneratedAt, r.Facts.CapturedAt, r.Facts.Pool.CapturedAt} {
		if now.IsZero() || at.IsZero() || at.After(now.Add(5*time.Second)) || now.Sub(at) > 30*time.Second {
			return fmt.Errorf("capacity source is stale or clock skewed")
		}
	}
	if r.Facts.Pool.CapturedAt.After(r.GeneratedAt) || r.Facts.CapturedAt.After(r.Facts.Pool.CapturedAt.Add(5*time.Second)) ||
		r.Facts.Pool.CapturedAt.Sub(r.Facts.CapturedAt) > 30*time.Second {
		return fmt.Errorf("capacity observation times contradict the read sequence")
	}
	return nil
}

type WorkerReservationHeadroom struct {
	Subject                    string `json:"subject"`
	RecordedRegistrationUsable bool   `json:"recorded_registration_usable"`
	LiveLeaseLimit             int64  `json:"live_lease_limit"`
	UnoccupiedReservations     int64  `json:"unoccupied_reservations"`
}

type CapacityObservation struct {
	Version                       int                         `json:"version"`
	Scope                         string                      `json:"scope"`
	RequestedAt                   time.Time                   `json:"requested_at"`
	ReceivedAt                    time.Time                   `json:"received_at"`
	ExpectedSource                CapacitySource              `json:"expected_source"`
	PeerCertificateDigest         string                      `json:"peer_certificate_digest"`
	ResponseDigest                string                      `json:"response_digest"`
	ResponseBytes                 []byte                      `json:"response_bytes_base64"`
	Response                      CapacityResponse            `json:"response"`
	PoolReservationHeadroom       int64                       `json:"pool_reservation_headroom"`
	WorkerReservationHeadroom     int64                       `json:"worker_reservation_headroom"`
	Workers                       []WorkerReservationHeadroom `json:"workers"`
	ServiceReadiness              string                      `json:"service_readiness"`
	WorkerProcessCapacityObserved bool                        `json:"worker_process_capacity_observed"`
	PublisherCapacityObserved     bool                        `json:"publisher_capacity_observed"`
	ExecutionAuthorized           bool                        `json:"execution_authorized"`
	Ready                         bool                        `json:"ready"`
	ProductionQualified           bool                        `json:"production_qualified"`
}

// ObserveCapacity consumes the original response from one pinned direct-mTLS
// GET. Neither a historical poll nor unoccupied lease budget means a Worker is
// running or can accept work. No result grants execution/recovery/READY.
func ObserveCapacity(raw []byte, peerDigest string, requested, received time.Time, expected CapacitySource) (CapacityObservation, error) {
	var zero CapacityObservation
	if expected.Validate() != nil || peerDigest != expected.ServerCertificateDigest || requested.IsZero() || received.Before(requested) || received.Sub(requested) > 30*time.Second {
		return zero, fmt.Errorf("bounded read and all externally expected source identities required")
	}
	var response CapacityResponse
	if err := strictjson.Decode(raw, &response); err != nil {
		return zero, err
	}
	if response.Source != expected || response.ValidateAt(received) != nil {
		return zero, fmt.Errorf("capacity response source, instance or freshness mismatch")
	}
	for _, at := range []time.Time{response.GeneratedAt, response.Facts.CapturedAt, response.Facts.Pool.CapturedAt} {
		if at.Before(requested.Add(-5 * time.Second)) {
			return zero, fmt.Errorf("capacity observation predates this request")
		}
	}
	result := CapacityObservation{
		Version: CapacityVersion, Scope: CapacityScope, RequestedAt: requested, ReceivedAt: received,
		ExpectedSource: expected, PeerCertificateDigest: peerDigest, ResponseDigest: canonical.BytesDigest(raw),
		ResponseBytes: bytes.Clone(raw), Response: response, ServiceReadiness: ServiceReadinessUnobserved,
		Workers: []WorkerReservationHeadroom{},
	}
	p := response.Facts.Pool
	result.PoolReservationHeadroom = p.Maximum - p.Acquired - p.Constructing
	for _, fact := range response.Facts.Workers {
		usable := fact.Recorded && fact.Protocol == workerqueue.Protocol && fact.Status == "ONLINE" && slices.Contains(fact.Grant.Profiles, fact.LastPollProfile)
		row := WorkerReservationHeadroom{Subject: fact.Grant.Subject, RecordedRegistrationUsable: usable, LiveLeaseLimit: 1}
		if usable {
			row.UnoccupiedReservations = 1 - fact.ActiveLeases
			result.WorkerReservationHeadroom += row.UnoccupiedReservations
		}
		result.Workers = append(result.Workers, row)
	}
	return result, nil
}

// Each wire fact must exist, including explicit zero/false/empty arrays. Only
// named nullable timestamps may be null; omission can never manufacture zero.
func decodeCapacityObject(data []byte, dst any, nullable ...string) error {
	if err := strictjson.Decode(data, dst); err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	typ := reflect.TypeOf(dst).Elem()
	for i := 0; i < typ.NumField(); i++ {
		key := strings.Split(typ.Field(i).Tag.Get("json"), ",")[0]
		raw, present := fields[key]
		if !present || (bytes.Equal(bytes.TrimSpace(raw), []byte("null")) && !slices.Contains(nullable, key)) {
			return fmt.Errorf("capacity fact %s is required", key)
		}
	}
	return nil
}

func (s *CapacitySource) UnmarshalJSON(data []byte) error {
	type wire CapacitySource
	var value wire
	if err := decodeCapacityObject(data, &value); err != nil {
		return err
	}
	*s = CapacitySource(value)
	return nil
}
func (s *WorkerCapacityGrant) UnmarshalJSON(data []byte) error {
	type wire WorkerCapacityGrant
	var value wire
	if err := decodeCapacityObject(data, &value); err != nil {
		return err
	}
	*s = WorkerCapacityGrant(value)
	return nil
}
func (s *WorkerCapacityFact) UnmarshalJSON(data []byte) error {
	type wire WorkerCapacityFact
	var value wire
	if err := decodeCapacityObject(data, &value, "last_poll_at"); err != nil {
		return err
	}
	*s = WorkerCapacityFact(value)
	return nil
}
func (s *QueueCapacityFact) UnmarshalJSON(data []byte) error {
	type wire QueueCapacityFact
	var value wire
	if err := decodeCapacityObject(data, &value, "oldest_pending_at"); err != nil {
		return err
	}
	*s = QueueCapacityFact(value)
	return nil
}
func (s *PoolCapacityFact) UnmarshalJSON(data []byte) error {
	type wire PoolCapacityFact
	var value wire
	if err := decodeCapacityObject(data, &value); err != nil {
		return err
	}
	*s = PoolCapacityFact(value)
	return nil
}
func (s *CapacityFacts) UnmarshalJSON(data []byte) error {
	type wire CapacityFacts
	var value wire
	if err := decodeCapacityObject(data, &value); err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	type wireAuthority OperationalStatus
	var authority wireAuthority
	if err := decodeCapacityObject(fields["authority"], &authority); err != nil {
		return err
	}
	// Keep the original operational status wire contract unchanged while
	// requiring every nested poll fact in this authenticated capacity response.
	var nested struct {
		Snapshot struct {
			WorkerPolls []json.RawMessage `json:"worker_polls"`
		} `json:"snapshot"`
	}
	if err := json.Unmarshal(fields["authority"], &nested); err != nil {
		return err
	}
	for _, raw := range nested.Snapshot.WorkerPolls {
		var poll WorkerPollObservation
		if err := decodeCapacityObject(raw, &poll); err != nil {
			return err
		}
	}
	*s = CapacityFacts(value)
	return nil
}
func (s *CapacityResponse) UnmarshalJSON(data []byte) error {
	type wire CapacityResponse
	var value wire
	if err := decodeCapacityObject(data, &value); err != nil {
		return err
	}
	*s = CapacityResponse(value)
	return nil
}
