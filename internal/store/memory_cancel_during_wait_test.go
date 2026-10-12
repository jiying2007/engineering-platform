package store

import (
 "context"
 "errors"
 "sync"
 "testing"
 "time"

 "github.com/jiying2007/engineering-platform/internal/action"
 "github.com/jiying2007/engineering-platform/internal/core"
)

// Return the observed error from BEFORE notifying the controller, so cancelling
// after firstChecked deterministically happens after the initial check, not by
// guessing scheduler timing or sleeping. RWMutex remains the one ledger lock.
type memoryCheckedContext struct {
 context.Context
 firstChecked chan struct{}
 once sync.Once
}
func (c *memoryCheckedContext) Err() error {
 err:=c.Context.Err()
 c.once.Do(func(){close(c.firstChecked)})
 return err
}

func TestMemoryCancellationWhileWaitingCannotReadOrMutate(t *testing.T) {
 cases:=[]struct{name string;call func(context.Context,*Memory)error;check func(*testing.T,*Memory)}{
  {"work-create",func(ctx context.Context,m *Memory)error{return m.CreateWorkContext(ctx,core.WorkItem{ID:"cancelled"})},func(t *testing.T,m *Memory){
   if _,err:=m.GetWork("cancelled");!errors.Is(err,ErrNotFound){t.Fatalf("cancelled write persisted: %v",err)}
  }},
  {"work-read",func(ctx context.Context,m *Memory)error{_,err:=m.GetWorkContext(ctx,"existing");return err},nil},
  {"recovery-read",func(ctx context.Context,m *Memory)error{_,err:=m.GetRecoveryContext(ctx);return err},nil},
  {"action-create",func(ctx context.Context,m *Memory)error{return m.CreateContext(ctx,action.Operation{ID:"cancelled",IdempotencyKey:"cancelled"})},func(t *testing.T,m *Memory){
   if _,err:=m.Get("cancelled");!errors.Is(err,action.ErrOperationAbsent){t.Fatalf("cancelled Action persisted: %v",err)}
  }},
 }
 for _,tc:=range cases{t.Run(tc.name,func(t *testing.T){
  m:=NewMemory()
  if err:=m.CreateWork(core.WorkItem{ID:"existing"});err!=nil{t.Fatal(err)}
  parent,cancel:=context.WithCancel(context.Background());defer cancel()
  ctx:=&memoryCheckedContext{Context:parent,firstChecked:make(chan struct{})}
  m.mu.Lock();locked:=true
  defer func(){if locked{m.mu.Unlock()}}()
  done:=make(chan error,1)
  go func(){done<-tc.call(ctx,m)}()
  select{case <-ctx.firstChecked:case <-time.After(2*time.Second):t.Fatal("initial cancellation check not reached")}
  cancel();m.mu.Unlock();locked=false
  select{
  case err:=<-done:if !errors.Is(err,context.Canceled){t.Fatalf("cancelled waiter completed: %v",err)}
  case <-time.After(2*time.Second):t.Fatal("waiter did not leave after lock release")
  }
  if tc.check!=nil{tc.check(t,m)}
 })}
}
