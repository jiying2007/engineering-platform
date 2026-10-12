package gateway

import (
 "context"
 "errors"
 "testing"

 "github.com/jiying2007/engineering-platform/internal/run"
 "github.com/jiying2007/engineering-platform/internal/session"
 "github.com/jiying2007/engineering-platform/internal/store"
)

type mismatchedSessionState struct{*store.Memory;change func(*run.Run,*session.Session)}
func(s mismatchedSessionState)GetExecutionContext(ctx context.Context,id string)(run.Run,session.Session,error){
 value,sess,err:=s.Memory.GetExecutionContext(ctx,id)
 if err==nil{s.change(&value,&sess)}
 return value,sess,err
}
// Preserve the useful historical epoch invariant in the single production
// Gateway. The unreferenced context-free authority package is not a second lane.
func TestGatewayRejectsCrossRunOrStaleSessionIdentity(t *testing.T){
 cases:=[]struct{name string;change func(*run.Run,*session.Session)}{
  {"stale-session",func(_ *run.Run,s *session.Session){s.ExecutionEpoch++}},
  {"cross-run-session",func(_ *run.Run,s *session.Session){s.RunID="other-run"}},
  {"cross-run-result",func(v *run.Run,_ *session.Session){v.ID="other-run"}},
 }
 for _,tc:=range cases{t.Run(tc.name,func(t *testing.T){
  _,memory,value:=setupAuthority(t,[]string{"ci.dispatch"})
  auth:=NewAuthority(mismatchedSessionState{Memory:memory,change:tc.change})
  if err:=auth.CheckRunEpoch(context.Background(),value.ID,value.CurrentEpoch);!errors.Is(err,ErrRunSessionEpochMismatch){t.Fatalf("invalid authority identity accepted: %v",err)}
 })}
}
