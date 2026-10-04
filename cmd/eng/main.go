package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/jiying2007/engineering-platform/internal/embedded"
	"github.com/jiying2007/engineering-platform/internal/material"
	"github.com/jiying2007/engineering-platform/internal/routing"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "help", "--help", "-h":
		if len(os.Args) != 2 {
			fmt.Fprintln(os.Stderr, "help takes no arguments")
			os.Exit(2)
		}
		usage()
	case "source-checkpoint":
		if err := sourceCheckpoint(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	case "runtime-isolation-probe":
		if err := runtimeIsolationProbe(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	case "run-control":
		if err := runControl(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	case "distribution-verify":
		if err := distributionVerify(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	case "api":
		if err := remoteAPI(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	case "import-ci-evidence":
		if err := importCIEvidence(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	case "offline-receipt-digest":
		if err := offlineReceiptDigest(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	case "import-offline-evidence":
		if err := importOfflineEvidence(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	case "relay-prequalification-pack":
		if err := relayPrequalificationPack(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	case "production-terminal-plan":
		if err := productionTerminalPlan(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	case "production-slo-report":
		if err := productionSLOReport(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	case "production-status":
		if err := productionStatus(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	case "production-preflight":
		if err := productionPreflight(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	case "relay-verify-qualification-kit":
		if err := relayVerifyQualificationKit(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	case "relay-qualification-kit":
		if err := relayQualificationKit(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	case "relay-runtime-handoff":
		if err := relayRuntimeHandoff(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	case "relay-live-manifest":
		if err := relayLiveManifest(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	case "relay-render-codex-config":
		if err := relayRenderCodexConfig(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	case "relay-prequalification":
		if err := relayPrequalification(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	case "codex-profile":
		if err := codexProfile(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	case "pilot-preflight":
		if err := pilotPreflight(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	case "codex-receipt-digest":
		if err := codexReceiptDigest(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	case "import-codex-evidence":
		if err := importCodexEvidence(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	case "git-change-manifest":
		if err := gitChangeManifest(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	case "import-git-change-evidence":
		if err := importGitChangeEvidence(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	case "capabilities":
		printJSON(map[string]any{
			"capabilities": embedded.Capabilities(),
			"skills":       embedded.Skills(),
		})
	case "route":
		if len(os.Args) < 4 {
			fmt.Fprintln(os.Stderr, "usage: eng route <task-type> <subsystem>")
			os.Exit(2)
		}
		route, err := routing.Resolve(os.Args[2], os.Args[3])
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		printJSON(route)
	case "material-check":
		if len(os.Args) != 3 {
			fmt.Fprintln(os.Stderr, "usage: eng material-check <manifest.json>")
			os.Exit(2)
		}
		data, err := os.ReadFile(os.Args[2])
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		var manifest material.Manifest
		if err := json.Unmarshal(data, &manifest); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		result := material.Evaluate(manifest)
		printJSON(result)
		if result.Status == material.Blocked {
			os.Exit(3)
		}
	default:
		fmt.Fprintln(os.Stderr, "unknown command:", os.Args[1])
		usage()
		os.Exit(2)
	}
}

func printJSON(value any) {
	b, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		panic(err)
	}
	fmt.Println(string(b))
}

func usage() {
	fmt.Println("eng <command>")
	fmt.Println("  source-checkpoint verify|restore <flags>  read back stopped source; never resume a model")
	fmt.Println("  runtime-isolation-probe                     verify local kernel process containment, without accounts")
	fmt.Println("  run-control inspect|status|steer|interrupt <flags>  explicit live control and receipt readback")
	fmt.Println("  distribution-verify --dir DIRECTORY       verify complete delivered executable roles and exact bytes")
	fmt.Println("  api GET|POST /api/v1/path [body.json]   call authenticated Control API")
	fmt.Println("  import-ci-evidence <flags>               verify GitHub CI provenance and register Evidence")
	fmt.Println("  offline-receipt-digest --run ID          compute immutable Worker execution receipt artifact digest")
	fmt.Println("  import-offline-evidence <flags>          register requirement-bound Worker execution Evidence")
	fmt.Println("  relay-prequalification-pack <flags>          build contract+assessment from non-secret relay policy inputs")
	fmt.Println("  production-terminal-plan                        emit immutable terminal maintenance acceptance contract")
	fmt.Println("  production-slo-report --observations FILE       summarize measured operational latency evidence without guessed targets")
	fmt.Println("  production-status [--require-ready]             read authenticated operational status from Control API")
	fmt.Println("  production-preflight --config FILE             verify Ubuntu production runtime baseline and external blockers")
	fmt.Println("  relay-verify-qualification-kit --dir DIR       independently verify exact immutable relay qualification bundle")
	fmt.Println("  relay-qualification-kit <flags>                materialize immutable offline relay qualification bundle")
	fmt.Println("  relay-runtime-handoff <flags>                  bind live manifest to exact Codex qualification/binary without model access")
	fmt.Println("  relay-live-manifest --contract FILE            freeze future read-only/no-tool live qualification plan")
	fmt.Println("  relay-render-codex-config --contract FILE --out FILE  render exact owner-private user-level Codex provider config")
	fmt.Println("  relay-prequalification --contract FILE       validate/digest relay provider contract without live account use")
	fmt.Println("  codex-profile --codex PATH --model MODEL derive exact Core-bound Codex profile/tool grant")
	fmt.Println("  pilot-preflight <flags>                   validate retained-pilot local/deployment readiness")
	fmt.Println("  codex-receipt-digest --run ID            compute immutable Core-bound Codex receipt digest")
	fmt.Println("  import-codex-evidence <flags>            verify Codex receipt/bundle and register Evidence")
	fmt.Println("  git-change-manifest <flags>              capture exact base/result Git tree provenance")
	fmt.Println("  import-git-change-evidence <flags>       re-capture Git trees and register requirement-bound Evidence")
	fmt.Println("  capabilities                         list embedded capabilities and skills")
	fmt.Println("  route <task-type> <subsystem>       resolve explicit M1 capability/skill route")
	fmt.Println("  material-check <manifest.json>      evaluate material readiness")
}
