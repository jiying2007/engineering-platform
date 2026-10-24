package access

import "slices"

// WorkerClaimGrant is a detached read-only view of the already loaded policy.
// Historical database registrations cannot add a currently authorized claimer.
type WorkerClaimGrant struct {
	Subject  string
	Profiles []string
	Prepares bool
}

func (p *Policy) WorkerClaimGrants() []WorkerClaimGrant {
	result := []WorkerClaimGrant{}
	if p == nil {
		return result
	}
	for _, principal := range p.principals {
		if !principal.Allows(WorkerPoll) {
			continue
		}
		profiles := make([]string, 0, len(principal.workerProfiles))
		for profile := range principal.workerProfiles {
			profiles = append(profiles, profile)
		}
		slices.Sort(profiles)
		result = append(result, WorkerClaimGrant{Subject: principal.Subject(), Profiles: profiles, Prepares: principal.Allows(WorkerPrepare)})
	}
	slices.SortFunc(result, func(a, b WorkerClaimGrant) int {
		if a.Subject < b.Subject {
			return -1
		}
		if a.Subject > b.Subject {
			return 1
		}
		return 0
	})
	return result
}
