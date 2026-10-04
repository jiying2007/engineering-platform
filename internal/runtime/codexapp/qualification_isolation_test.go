package codexapp

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/jiying2007/engineering-platform/internal/canonical"
	runtimeprovider "github.com/jiying2007/engineering-platform/internal/runtime"
)

func TestQualificationEnvironmentStableAndReceiptFailClosed(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux qualification")
	}
	a, err := qualificationEnvironmentDigest()
	if err != nil {
		t.Fatal(err)
	}
	b, err := qualificationEnvironmentDigest()
	if err != nil || a != b || !canonical.ValidDigest(a) {
		t.Fatal(a, b, err)
	}
	root := t.TempDir()
	bin := filepath.Join(root, "fixture")
	if err := os.WriteFile(bin, []byte("TEST ONLY NOT A CODEX BINARY"), 0700); err != nil {
		t.Fatal(err)
	}
	good := executableQualificationFixture(t, bin, "0.157.1", "test-model")
	for _, mutate := range []func(*QualificationReceipt){
		func(r *QualificationReceipt) { r.SchemaVersion = 2 },
		func(r *QualificationReceipt) { r.CompatibilityContractVersion = 1 },
		func(r *QualificationReceipt) { r.IsolationMechanism = "plain-process" },
		func(r *QualificationReceipt) { r.IsolationEnvironmentDigest = "" },
		func(r *QualificationReceipt) { r.IsolatedEngineeringStartup = false },
		func(r *QualificationReceipt) { r.NamespaceInitReaped = false },
		func(r *QualificationReceipt) { r.ModelTurnExecuted = true },
		func(r *QualificationReceipt) { r.CredentialUsed = true },
	} {
		bad := good
		mutate(&bad)
		if bad.Validate() == nil {
			t.Fatalf("unsafe receipt admitted: %+v", bad)
		}
	}
}

func TestQualificationOnlyProviderRefusesEveryCredentialSurface(t *testing.T) {
	root := t.TempDir()
	bin := filepath.Join(root, "codex")
	_ = os.WriteFile(bin, []byte("#!/bin/sh\nexit 0\n"), 0700)
	data, _ := os.ReadFile(bin)
	for _, extra := range []string{"OPENAI_API_KEY=TEST_ONLY", "OPENAI_BASE_URL=https://example.invalid", "OPENAI_FEDERATION_RULE_ID=test", "OPENAI_IDENTITY_TOKEN_FILE=/not-read", "OPENAI_WORKLOAD_IDENTITY_CONTEXT=test"} {
		t.Run(strings.Split(extra, "=")[0], func(t *testing.T) {
			work, home := filepath.Join(t.TempDir(), "work"), filepath.Join(t.TempDir(), "home")
			_ = os.Mkdir(work, 0700)
			_ = os.Mkdir(home, 0700)
			p, _ := NewPinnedProvider(bin, canonical.BytesDigest(data))
			p.qualificationOnly = true
			p.engineeringMode = true
			if _, err := p.Command(context.Background(), runtimeprovider.LaunchSpec{Dir: work, Env: []string{"HOME=" + home, extra}}); err == nil {
				t.Fatal("credential or endpoint admitted")
			}
			if entries, _ := os.ReadDir(home); len(entries) != 0 {
				t.Fatal("rejected probe changed HOME")
			}
		})
	}
}

// This is a local protocol subprocess, not a real Codex or provider qualification.
// The canonical qualification job separately executes the actual native binary.
func TestIsolatedQualificationActualProtocolSubprocess(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux process namespaces required")
	}
	for _, mode := range []string{"success", "bad-thread", "bad-initialize"} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			work, home := filepath.Join(root, "work"), filepath.Join(root, "home")
			_ = os.Mkdir(work, 0700)
			_ = os.Mkdir(home, 0700)
			executable := filepath.Join(root, "codex-test-protocol")
			code := fmt.Sprintf(`#!/usr/bin/python3
import json,os,pathlib,sys
assert sys.argv[1:]==['app-server','--stdio']
assert os.getpid()==1, 'probe is not namespace init'
assert not any(k.startswith(('OPENAI_','GH_','GITHUB_')) for k in os.environ)
config=pathlib.Path(os.environ['CODEX_HOME'],'config.toml').read_text()
assert 'sandbox_mode = "workspace-write"' in config
assert 'network_access = false' in config
assert not pathlib.Path(os.environ['CODEX_HOME'],'auth.json').exists()
for raw in sys.stdin:
    m=json.loads(raw); method=m['method']
    with open('calls.txt','a') as f:
        f.write(method+'\n'); f.flush();os.fsync(f.fileno())
    assert method in ['initialize','initialized','thread/start'],method
    if method=='initialized': continue
    if method=='thread/start':
        assert m['params']['sandbox']=='workspace-write'
        assert m['params']['approvalPolicy']=='never'
        assert m['params']['ephemeral'] is True
        result={'thread':{'id':'isolated-test-thread'}} if %q!='bad-thread' else {'thread':{}}
    else:
        if %q=='bad-initialize':
            print(json.dumps({'id':m['id'],'error':{'code':-1,'message':'test rejection'}}),flush=True);continue
        result={}
    print(json.dumps({'id':m['id'],'result':result}),flush=True)
`, mode, mode)
			if err := os.WriteFile(executable, []byte(code), 0700); err != nil {
				t.Fatal(err)
			}
			err := qualifyIsolatedEngineeringStartup(context.Background(), executable, canonical.BytesDigest([]byte(code)), "test-only-model", work, home)
			if err != nil && strings.Contains(err.Error(), "namespace unavailable") && os.Getenv("EP_REQUIRE_PID_NAMESPACE_TESTS") != "1" {
				t.Skip(err)
			}
			if (err == nil) != (mode == "success") {
				t.Fatalf("mode %s: %v", mode, err)
			}
			calls, e := os.ReadFile(filepath.Join(work, "calls.txt"))
			if e != nil {
				t.Fatal(e, err)
			}
			if mode == "success" && string(calls) != "initialize\ninitialized\nthread/start\n" {
				t.Fatalf("unexpected probe methods: %s", calls)
			}
			if strings.Contains(string(calls), "turn/") || strings.Contains(string(calls), "account/") {
				t.Fatal("probe attempted model or account effect")
			}
		})
	}
}

func TestQualifyCompleteFixtureStableAcrossNamespaces(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux process namespaces required")
	}
	root := t.TempDir()
	bin := filepath.Join(root, "codex-full-probe")
	script := `#!/usr/bin/python3
import json,os,pathlib,sys
args=sys.argv[1:]
if args==['--version']:
 print('codex-cli 0.157.1');sys.exit(0)
if args[:2]==['app-server','generate-json-schema']:
 out=pathlib.Path(args[args.index('--out')+1])
 thread={'type':'object','properties':{'approvalPolicy':{},'sandbox':{}}}
 if '--experimental' in args:thread['properties']['permissions']={}
 (out/'ThreadStartParams.json').write_text(json.dumps(thread))
 (out/'Enums.json').write_text(json.dumps({'sandbox':['read-only','workspace-write','danger-full-access'],'approval':['never','on-request']}))
 sys.exit(0)
if args==['features','list']:
 config=pathlib.Path(os.environ['CODEX_HOME'],'config.toml').read_text()
 shell='true' if 'shell_tool = true' in config else 'false'
 print('shell_tool stable '+shell+'\nview_image stable false');sys.exit(0)
assert args==['app-server','--stdio'] and os.getpid()==1
assert not pathlib.Path(os.environ['CODEX_HOME'],'auth.json').exists()
for line in sys.stdin:
 m=json.loads(line)
 assert m['method'] in ['initialize','initialized','thread/start']
 if m['method']=='initialized':continue
 if m['method']=='thread/start':
  assert m['params']['sandbox']=='workspace-write'
  result={'thread':{'id':'synthetic-no-model'}}
 else:result={}
 print(json.dumps({'id':m['id'],'result':result}),flush=True)
`
	if err := os.WriteFile(bin, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	a, err := Qualify(context.Background(), bin, "test-no-model")
	if err != nil && strings.Contains(err.Error(), "namespace unavailable") && os.Getenv("EP_REQUIRE_PID_NAMESPACE_TESTS") != "1" {
		t.Skip(err)
	}
	if err != nil {
		t.Fatal(err)
	}
	b, err := Qualify(context.Background(), bin, "test-no-model")
	if err != nil || a != b {
		t.Fatal("qualification identity drift", a, b, err)
	}
	if !a.IsolatedEngineeringStartup || !a.NamespaceInitReaped || a.ModelTurnExecuted || a.CredentialUsed {
		t.Fatal(a)
	}
}
