package rcdelivery

import (
 "bytes"
 "errors"
 "strings"

 "github.com/jiying2007/engineering-platform/internal/strictjson"
)

const readinessStart = "<!-- engineering-platform-readiness\n"
const readinessEnd = "\n-->"

// ReadinessSnapshot is read from the single implementation-status authority.
// It separates internal blockers from production acceptance and deferred
// credential alternatives. A digest or green CI cannot promote any of them.
type ReadinessSnapshot struct {
 Version int `json:"version"`
 InternalReadiness string `json:"internal_readiness"`
 InternalBlockers []int `json:"internal_blockers"`
 ExternalQualificationGates []int `json:"external_qualification_gates"`
 DeferredCredentialGates []int `json:"deferred_credential_gates"`
}

func (s ReadinessSnapshot) Validate() error {
 if s.Version!=1 || s.InternalBlockers==nil || s.ExternalQualificationGates==nil || s.DeferredCredentialGates==nil || len(s.ExternalQualificationGates)==0 {
  return errors.New("explicit readiness version and gate sets required")
 }
 switch s.InternalReadiness {
 case "BLOCKED":
  if len(s.InternalBlockers)==0{return errors.New("blocked readiness requires internal blockers")}
 case "CANDIDATE_ONLY":
  if len(s.InternalBlockers)!=0{return errors.New("internal blockers prohibit candidate readiness")}
 default:return errors.New("unsupported or overclaimed internal readiness")
 }
 seen:=map[int]bool{}
 for _,group:=range [][]int{s.InternalBlockers,s.ExternalQualificationGates,s.DeferredCredentialGates}{
  previous:=0
  for _,issue:=range group{
   if issue<=previous || seen[issue]{return errors.New("readiness issues must be positive, unique and sorted within each gate set")}
   previous=issue;seen[issue]=true
  }
 }
 return nil
}

// ReadinessFromStatus has no default-to-ready path. This remains a declared
// repository posture, not a live GitHub issue query or independent review.
func ReadinessFromStatus(status []byte) (ReadinessSnapshot,error) {
 var zero ReadinessSnapshot
 if len(status)==0 || len(status)>strictjson.MaxBytes || !bytes.HasPrefix(status,[]byte("# Implementation Status\n")){
  return zero,errors.New("bounded implementation-status document required")
 }
 source:=string(status)
 if strings.Count(source,"<!-- engineering-platform-readiness")!=1 || strings.Count(source,readinessStart)!=1{
  return zero,errors.New("one exact implementation readiness block required")
 }
 start:=strings.Index(source,readinessStart)+len(readinessStart)
 end:=strings.Index(source[start:],readinessEnd)
 if end<0{return zero,errors.New("unterminated implementation readiness block")}
 raw:=[]byte(source[start:start+end])
 if len(raw)>8192{return zero,errors.New("readiness block exceeds limit")}
 var snapshot ReadinessSnapshot
 if err:=strictjson.Decode(raw,&snapshot);err!=nil{return zero,errors.New("invalid or ambiguous implementation readiness block")}
 if err:=snapshot.Validate();err!=nil{return zero,err}
 return snapshot,nil
}
