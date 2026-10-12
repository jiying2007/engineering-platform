package postgres

import (
 "context"
 "errors"
 "testing"
 "time"

 "github.com/jackc/pgx/v5/pgxpool"
 "github.com/jiying2007/engineering-platform/internal/action"
 "github.com/jiying2007/engineering-platform/internal/core"
 "github.com/jiying2007/engineering-platform/internal/gateway"
)

type poolWaitProvider struct{calls int}
func(p *poolWaitProvider)Dispatch(context.Context,action.Request)(action.DispatchResult,error){p.calls++;return action.DispatchResult{},errors.New("unexpected provider dispatch")}
func(p *poolWaitProvider)Reconcile(context.Context,action.Operation)(action.ReconcileResult,error){p.calls++;return action.ReconcileResult{},errors.New("unexpected provider observation")}

// A PostgreSQL server-side statement deadline starts too late to bound a pool
// acquisition. Hold the only connection until the caller has returned; verify
// that cancelled waiters never reach SQL or a Provider and the pool remains usable.
// The watchdog is a test liveness budget, not a selected-host performance SLO.
func TestPostgresCallerContextBoundsPoolWait(t *testing.T){
 original:=newIsolatedIntegrationStore(t)
 config:=original.pool.Config();config.MaxConns=1;config.MinConns=0
 setup,stop:=context.WithTimeout(context.Background(),10*time.Second);defer stop()
 pool,err:=pgxpool.NewWithConfig(setup,config);if err!=nil{t.Fatal(err)}
 t.Cleanup(pool.Close)
 scoped:=New(pool);provider:=&poolWaitProvider{};auth:=gateway.NewAuthority(scoped)
 service:=action.NewService(auth,auth,provider,scoped)
 cases:=[]struct{name string;call func(context.Context)error}{
  {"work-read",func(ctx context.Context)error{_,err:=scoped.GetWorkContext(ctx,"pool-cancelled");return err}},
  {"work-write",func(ctx context.Context)error{return scoped.CreateWorkContext(ctx,core.WorkItem{ID:"pool-cancelled",Title:"cancelled",HumanOwner:"test",State:core.WorkDraft,CreatedAt:time.Now().UTC()})}},
  {"recovery-read",func(ctx context.Context)error{_,err:=scoped.GetRecoveryContext(ctx);return err}},
  {"recovery-begin",func(ctx context.Context)error{_,err:=scoped.BeginRecoveryContext(ctx,0);return err}},
  {"action-read",func(ctx context.Context)error{_,err:=scoped.GetContext(ctx,"pool-operation");return err}},
  {"idempotency-read",func(ctx context.Context)error{_,err:=scoped.GetByIdempotencyKeyContext(ctx,"pool-idempotency");return err}},
  {"action-execute",func(ctx context.Context)error{_,err:=service.Execute(ctx,action.Request{ID:"pool-operation",RunID:"no-run",ExecutionEpoch:1,Action:"ci.dispatch",RiskClass:action.ControlledMutation,Capability:"ci.dispatch",IdempotencyKey:"pool-idempotency",ParametersDigest:"sha256:pool-probe",RequestedBy:"test"});return err}},
 }
 for _,tc:=range cases{t.Run(tc.name,func(t *testing.T){
  held,err:=pool.Acquire(setup);if err!=nil{t.Fatal(err)};defer held.Release()
  ctx,cancel:=context.WithTimeout(context.Background(),100*time.Millisecond);defer cancel()
  done:=make(chan error,1);go func(){done<-tc.call(ctx)}()
  select{
  case err:=<-done:if !errors.Is(err,context.DeadlineExceeded){t.Fatalf("pool waiter ignored caller deadline: %v",err)}
  case <-time.After(3*time.Second):
   // Release on failure so a broken background-context call can leave without
   // trapping pool.Close. Never silently convert this failure to PASS.
   held.Release();select{case <-done:case <-time.After(3*time.Second):}
   t.Fatal("pool wait did not stop while the connection remained held")
  }
 })}
 if provider.calls!=0{t.Fatalf("cancelled admission reached provider %d times",provider.calls)}
 verify,cancel:=context.WithTimeout(context.Background(),5*time.Second);defer cancel()
 var works,events,operations int
 if err:=original.pool.QueryRow(verify,"SELECT (SELECT count(*) FROM work_items),(SELECT count(*) FROM audit_events),(SELECT count(*) FROM external_operations)").Scan(&works,&events,&operations);err!=nil{t.Fatal(err)}
 if works!=0 || events!=0 || operations!=0{t.Fatalf("cancelled pool wait left authority residue: works=%d audit=%d operations=%d",works,events,operations)}
 recovery,err:=scoped.GetRecoveryContext(verify)
 if err!=nil || recovery.Epoch!=0{t.Fatalf("pool unusable or cancelled Recovery committed: %+v %v",recovery,err)}
 if err:=scoped.CreateWorkContext(verify,core.WorkItem{ID:"pool-reused",Title:"valid after cancellation",HumanOwner:"test",State:core.WorkDraft,CreatedAt:time.Now().UTC()});err!=nil{t.Fatalf("cancelled wait poisoned pool: %v",err)}
}
