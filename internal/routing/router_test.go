package routing

import (
	"reflect"
	"testing"
)

func TestLinuxDebugRoute(t *testing.T) {
	r, err := Resolve("DEBUG", "Linux UBI storage")
	if err != nil {
		t.Fatal(err)
	}
	if len(r.SkillIDs) != 3 || r.SkillIDs[2] != "linux-bsp-debug" {
		t.Fatalf("unexpected route: %#v", r)
	}
}

func TestSubsystemAwareIntegrationRoutes(t *testing.T) {
	for _, taskType := range []string{"FEATURE", "BRINGUP", "COMPATIBILITY"} {
		for _, tc := range []struct {
			name       string
			subsystem  string
			capability string
			skill      string
		}{
			{name: "linux", subsystem: "Linux UBI storage", capability: "embedded.linux-bsp", skill: "linux-bsp-integration"},
			{name: "mcu", subsystem: "MCU motor control", capability: "embedded.mcu-rtos", skill: "mcu-rtos-integration"},
		} {
			t.Run(taskType+"/"+tc.name, func(t *testing.T) {
				route, err := Resolve(taskType, tc.subsystem)
				if err != nil {
					t.Fatal(err)
				}
				wantCapabilities := []string{"embedded.architecture", "embedded.driver-component", tc.capability}
				wantSkills := []string{"material-readiness", "architecture-impact-analysis", "interface-contract-review", "driver-integration-review", tc.skill}
				if !reflect.DeepEqual(route.CapabilityIDs, wantCapabilities) || !reflect.DeepEqual(route.SkillIDs, wantSkills) {
					t.Fatalf("unexpected route: %#v", route)
				}
			})
		}
	}
}

func TestGenericFeatureRouteIsUnchanged(t *testing.T) {
	route, err := Resolve("feature", "generic subsystem")
	if err != nil {
		t.Fatal(err)
	}
	want := Route{
		TaskType:      "FEATURE",
		CapabilityIDs: []string{"embedded.architecture", "embedded.driver-component"},
		SkillIDs:      []string{"material-readiness", "architecture-impact-analysis", "interface-contract-review", "driver-integration-review"},
	}
	if !reflect.DeepEqual(route, want) {
		t.Fatalf("unexpected route: %#v", route)
	}
}

func TestDebugRoutesAreUnchanged(t *testing.T) {
	tests := []struct {
		subsystem string
		want      Route
	}{
		{
			subsystem: "Linux UBI storage",
			want:      Route{TaskType: "DEBUG", CapabilityIDs: []string{"embedded.debug-reliability", "embedded.linux-bsp"}, SkillIDs: []string{"material-readiness", "log-triage", "linux-bsp-debug"}},
		},
		{
			subsystem: "MCU motor control",
			want:      Route{TaskType: "DEBUG", CapabilityIDs: []string{"embedded.debug-reliability", "embedded.mcu-rtos"}, SkillIDs: []string{"material-readiness", "log-triage", "mcu-rtos-debug"}},
		},
		{
			subsystem: "generic subsystem",
			want:      Route{TaskType: "DEBUG", CapabilityIDs: []string{"embedded.debug-reliability"}, SkillIDs: []string{"material-readiness", "log-triage"}},
		},
	}
	for _, tc := range tests {
		route, err := Resolve("DEBUG", tc.subsystem)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(route, tc.want) {
			t.Fatalf("unexpected route for %q: %#v", tc.subsystem, route)
		}
	}
}

func TestUnsupportedTaskFailsClosed(t *testing.T) {
	if _, err := Resolve("MAGIC", "anything"); err == nil {
		t.Fatal("expected unsupported task error")
	}
}

func TestStructuredTargetRoutingWithOpaqueChipIdentity(t *testing.T) {
	for _, tc := range []struct {
		task, subsystem, targetID, platform, want string
	}{
		{"DEBUG", "SSC305", "ssc305", PlatformLinuxBSP, "linux-bsp-debug"},
		{"FEATURE", "MM32SPIN023C", "mm32spin023c", PlatformMCURTOS, "mcu-rtos-integration"},
		{"DEBUG", "GD32L235", "gd32l235", PlatformMCURTOS, "mcu-rtos-debug"},
		{"FEATURE", "opaque board", "board-rev-a", PlatformLinuxBSP, "linux-bsp-integration"},
	} {
		t.Run(tc.targetID, func(t *testing.T) {
			got, err := ResolveForTarget(tc.task, tc.subsystem, &TargetContext{TargetID: tc.targetID, Platform: tc.platform})
			if err != nil {
				t.Fatal(err)
			}
			if got.SkillIDs[len(got.SkillIDs)-1] != tc.want {
				t.Fatalf("unexpected route: %#v", got)
			}
		})
	}
}

func TestStructuredTargetContradictionsFailClosed(t *testing.T) {
	for _, tc := range []struct {
		subsystem string
		target    TargetContext
	}{
		{"linux storage", TargetContext{TargetID: "mcu-1", Platform: PlatformMCURTOS}},
		{"mcu motor", TargetContext{TargetID: "soc-1", Platform: PlatformLinuxBSP}},
		{"Linux MCU", TargetContext{TargetID: "board-a", Platform: PlatformLinuxBSP}},
		{"unknown", TargetContext{TargetID: "", Platform: PlatformLinuxBSP}},
		{"unknown", TargetContext{TargetID: "board-a", Platform: "unknown"}},
		{"unknown", TargetContext{TargetID: " board-a", Platform: PlatformLinuxBSP}},
		{"unknown", TargetContext{TargetID: "board-a\n", Platform: PlatformLinuxBSP}},
	} {
		if got, err := ResolveForTarget("DEBUG", tc.subsystem, &tc.target); err == nil {
			t.Fatalf("accepted contradictory target: %#v", got)
		}
	}
	legacy, err := Resolve("DEBUG", "generic subsystem")
	if err != nil {
		t.Fatal(err)
	}
	absent, err := ResolveForTarget("DEBUG", "generic subsystem", nil)
	if err != nil || !reflect.DeepEqual(legacy, absent) {
		t.Fatalf("absent structured target changed existing route: %#v %v", absent, err)
	}
}
