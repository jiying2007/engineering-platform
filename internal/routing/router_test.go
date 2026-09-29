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
