package access

import "fmt"

const (
	WorkerPoll    = "worker:poll"
	WorkerReport  = "worker:report"
	WorkerPrepare = "worker:prepare"
)

func (id Identity) AllowsWorkerProfile(profile string) bool { return id.workerProfiles[profile] }
func configureWorkerProfiles(spec PrincipalSpec, id *Identity) error {
	id.workerProfiles = map[string]bool{}
	if len(spec.WorkerProfiles) > 64 {
		return fmt.Errorf("too many worker profile grants")
	}
	for _, profile := range spec.WorkerProfiles {
		if !namePattern.MatchString(profile) || id.workerProfiles[profile] {
			return fmt.Errorf("invalid or duplicate worker profile")
		}
		id.workerProfiles[profile] = true
	}
	if (id.Allows(WorkerPoll) || id.Allows(WorkerReport)) != (len(id.workerProfiles) > 0) {
		return fmt.Errorf("worker capabilities require explicit worker profiles")
	}
	if id.Allows(WorkerPrepare) && (!id.Allows(WorkerPoll) || !id.Allows(WorkerReport)) {
		return fmt.Errorf("worker preparation requires poll and report grants")
	}
	return nil
}

// Register the compiled preparation vocabulary before any policy is loaded.
// Policies remain immutable after construction; no request can add a capability.
func init() { capabilities[WorkerPrepare] = true }

// Admission and preparation are alternative consumers, not serial stages. A
// plain admission report terminally consumes its inbox row without materializing
// source. Opposite kinds of polling principal must therefore never share a
// routing profile. Same-kind replicas and report-only identities remain valid.
func validateWorkerClaimLanes(principals map[string]Identity) error {
	lanes := make(map[string]bool)
	for _, id := range principals {
		if !id.Allows(WorkerPoll) {
			continue
		}
		prepare := id.Allows(WorkerPrepare)
		for profile := range id.workerProfiles {
			if prior, exists := lanes[profile]; exists && prior != prepare {
				return fmt.Errorf("worker profile mixes admission-only and preparation-capable claimers")
			}
			lanes[profile] = prepare
		}
	}
	return nil
}
