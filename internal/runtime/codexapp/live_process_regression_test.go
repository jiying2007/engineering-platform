package codexapp

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jiying2007/engineering-platform/internal/canonical"
	"github.com/jiying2007/engineering-platform/internal/testsupport"
)

// This executable only exchanges fixture JSON over inherited local pipes. Its
// double-forked child self-expires after three seconds even on a broken launcher.
// No credential is authenticated and no provider/model is called.
const liveDescendantFixture = `#!/usr/bin/python3
import json, os, pathlib, sys, time
if sys.argv[1:] == ['--version']:
 print('codex-cli 0.157.1');sys.exit(0)
root=pathlib.Path(__file__).parent
heartbeat=root/'heartbeat'
mode=(root/'mode').read_text()
def reply(id,result): print(json.dumps({'id':id,'result':result}),flush=True)
for line in sys.stdin:
 m=json.loads(line);method=m.get('method');id=m.get('id')
 if method=='initialized':continue
 if method=='initialize':reply(id,{'userAgent':'synthetic-fixture'})
 elif method=='account/rateLimits/read':reply(id,{'rateLimits':{'primary':None}})
 elif method=='thread/start':reply(id,{'thread':{'id':'thread-fixture'}})
 elif method=='turn/start':
  if os.fork()==0:
   os.setsid()
   if os.fork()!=0:os._exit(0)
   null=os.open('/dev/null',os.O_RDWR)
   for fd in (0,1,2):os.dup2(null,fd)
   if null>2:os.close(null)
   with heartbeat.open('ab',buffering=0) as out:
    until=time.monotonic()+3
    while time.monotonic()<until:
     out.write(b'x');time.sleep(.005)
   os._exit(0)
  until=time.monotonic()+2
  while (not heartbeat.exists() or heartbeat.stat().st_size==0) and time.monotonic()<until:time.sleep(.002)
  if mode=='cancel':
   time.sleep(3);sys.exit(2)
  if mode=='rpc-error':
   print(json.dumps({'id':id,'error':{'code':-32000,'message':'synthetic turn rejection'}}),flush=True)
   continue
  reply(id,{'turn':{'id':'turn-fixture'}})
  print(json.dumps({'method':'item/completed','params':{'threadId':'thread-fixture','turnId':'turn-fixture','item':{'id':'message-fixture','type':'agentMessage','text':'engineering-platform live qualification'}}}),flush=True)
  print(json.dumps({'method':'turn/completed','params':{'threadId':'thread-fixture','turn':{'id':'turn-fixture','status':'completed'}}}),flush=True)
 else:sys.exit(2)
`

func TestLiveProbeStopsDetachedDescendants(t *testing.T) {
	testsupport.RequireProcessNamespaces(t)
	for _, lane := range []string{"saved", "wif"} {
		for _, mode := range []string{"completed", "cancel", "rpc-error"} {
			t.Run(lane+"/"+mode, func(t *testing.T) {
				root := t.TempDir()
				work, home, credentials := filepath.Join(root, "work"), filepath.Join(root, "home"), filepath.Join(root, "credentials")
				for _, dir := range []string{work, home, credentials} {
					if err := os.Mkdir(dir, 0700); err != nil {
						t.Fatal(err)
					}
				}
				login, token := filepath.Join(credentials, "auth.json"), filepath.Join(credentials, "token")
				if err := os.WriteFile(login, []byte(`{"synthetic_fixture":"not_a_real_credential"}`), 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(token, []byte("fixture-only-not-a-real-identity-token"), 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(root, "mode"), []byte(mode), 0600); err != nil {
					t.Fatal(err)
				}
				executable := probeFixture(t, root, liveDescendantFixture)
				ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
				defer cancel()
				type outcome struct {
					receipt LiveReceipt
					err     error
				}
				done := make(chan outcome, 1)
				go func() {
					var r LiveReceipt
					var err error
					if lane == "wif" {
						r, err = LiveWIFProbe(ctx, executable, canonical.BytesDigest([]byte(liveDescendantFixture)), work, home, "fixture-rule", token, "", "fixture-model")
					} else {
						r, err = LiveSavedLoginProbe(ctx, executable, canonical.BytesDigest([]byte(liveDescendantFixture)), work, home, login, "fixture-model")
					}
					done <- outcome{r, err}
				}()
				heartbeat := filepath.Join(root, "heartbeat")
				until := time.Now().Add(2 * time.Second)
				for {
					if b, err := os.ReadFile(heartbeat); err == nil && len(b) != 0 {
						break
					}
					if time.Now().After(until) {
						cancel()
						got := <-done
						t.Fatal("fixture descendant never started", got.err)
					}
					time.Sleep(2 * time.Millisecond)
				}
				if mode == "cancel" {
					cancel()
				}
				var got outcome
				select {
				case got = <-done:
				case <-time.After(2 * time.Second):
					cancel()
					t.Fatal("live probe did not stop within fixture budget")
				}
				if mode == "completed" {
					if got.err != nil || got.receipt.Validate() != nil || got.receipt.SchemaVersion != 2 {
						t.Fatalf("completed live probe lost its compatible receipt: %+v %v", got.receipt, got.err)
					}
				} else if got.err == nil || got.receipt.SchemaVersion != 0 || errors.Is(got.err, ErrProbeStopUnconfirmed) {
					t.Fatalf("failure did not retain its cause with confirmed cleanup: %+v %v", got.receipt, got.err)
				}
				assertProbeHeartbeatStopped(t, heartbeat)
			})
		}
	}
}
