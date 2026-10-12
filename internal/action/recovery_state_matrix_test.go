package action

import (
 "errors"
 "reflect"
 "testing"
 "time"
)

// Regression of the existing normal transition boundary, not authorization
// for Recovery abandonment or evidence of original dispatcher quiescence.
func TestAllActionStatePairsKeepNoReplayBoundary(t *testing.T){
 states:=[]State{Planned,Dispatched,Confirmed,Unknown,Reconciling,SafeToRetry,Manual,AbandonedReconciled,State("UNRECOGNIZED")}
 allowed:=map[[2]State]bool{
  {Planned,Dispatched}:true,{Dispatched,Confirmed}:true,{Dispatched,Unknown}:true,
  {Unknown,Reconciling}:true,{Reconciling,Confirmed}:true,{Reconciling,Manual}:true,
 }
 beforeTime:=time.Unix(100,0).UTC();afterTime:=time.Unix(101,0).UTC()
 for _,from:=range states{for _,to:=range states{t.Run(string(from)+"-to-"+string(to),func(t *testing.T){
  op:=Operation{ID:"operation",RunID:"run",ExecutionEpoch:3,RecoveryEpoch:7,IdempotencyKey:"reserved",RequestDigest:"unchanged",State:from,UpdatedAt:beforeTime}
  original:=op;err:=op.Transition(to,afterTime)
  if !allowed[[2]State{from,to}]{
   if !errors.Is(err,ErrInvalidTransition) || !reflect.DeepEqual(op,original){t.Fatal("forbidden transition mutated authority")}
   return
  }
  if err!=nil || op.State!=to || !op.UpdatedAt.Equal(afterTime){t.Fatal("permitted transition changed")}
  op.State=original.State;op.UpdatedAt=original.UpdatedAt
  if !reflect.DeepEqual(op,original){t.Fatal("transition rewrote frozen identity or idempotency reservation")}
 })}}
}
