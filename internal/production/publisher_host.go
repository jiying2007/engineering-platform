package production

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/jiying2007/engineering-platform/internal/githubpublish"
)

// Check the actual v2 service contract, not the old in-process publisher config.
// Check() already resolves four distinct non-root service identities. This
// helper validates filesystem ownership/access only; it never publishes.
func checkPublisherPolicyAndWitness(plan githubpublish.PlanConfiguration, pub githubpublish.ServiceConfiguration, uid int, groups map[uint32]bool) error {
	if plan.Version != 1 || pub.Version != 2 || len(plan.Targets) == 0 || len(plan.Targets) != len(pub.Targets) || plan.ArtifactRoot != pub.ArtifactRoot {
		return fmt.Errorf("publisher plan/service v2 policy mismatch")
	}
	for i, target := range plan.Targets {
		if target != pub.Targets[i] {
			return fmt.Errorf("publisher target policy mismatch")
		}
	}
	for _, path := range []string{pub.ArtifactRoot, pub.WitnessRoot} {
		if err := safeDirectory(path); err != nil {
			return err
		}
	}
	// Neither Worker-visible artifacts nor an ancestor of that tree may serve as
	// the Publisher's independent private evidence directory.
	for _, pair := range [][2]string{{pub.ArtifactRoot, pub.WitnessRoot}, {pub.WitnessRoot, pub.ArtifactRoot}} {
		relative, err := filepath.Rel(pair[0], pair[1])
		if err != nil || (relative != ".." && !strings.HasPrefix(relative, ".."+string(os.PathSeparator))) {
			return fmt.Errorf("publisher artifact and witness roots must be disjoint")
		}
	}
	info, err := os.Lstat(pub.WitnessRoot)
	if err != nil || !info.IsDir() || info.Mode().Perm() != 0700 || info.Mode()&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 {
		return fmt.Errorf("private Publisher-owned 0700 witness directory required")
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || int(stat.Uid) != uid {
		return fmt.Errorf("Publisher must own its independent witness directory")
	}
	if err := servicePathAccess(pub.WitnessRoot, uid, groups, 7); err != nil {
		return err
	}
	return nil
}
