# Caller and maintenance Context inventory — current source slice

Base: `c449d7c3f1bfaab3f9f07b61d1992d7248c6d285`, plus the scoped Publisher
Git and v2 host-admission corrections in this change. This closes the *enumeration* of the current
direct constructor candidates; it does not close #229, infer selected-host
capacity, or prove all indirect/aliased Context propagation.

Production `cmd` / `internal` Go source digest from the existing scanner:

`sha256:8923a9f32a8eabd1a4516f34605ac47932347081b510b23322ce9fae67c649ae`

## Reviewed categories

| Category | Direct call sites |
| --- | ---: |
| Explicitly timed command root | 27 |
| Signal-owned command/service root | 10 |
| Command with downstream budget | 2 |
| Independent bounded cleanup/settlement | 15 |
| Concrete convenience wrapper | 75 |
| Test support helper | 3 |

Command roots either own a signal lifetime or have explicit operation budgets.
The two downstream-budget calls are `processscope.Probe` (its existing bounded
probe/cleanup) and `controlclient.Raw` (its existing independent HTTP/dial/TLS
budgets). Daemon signal lifetime is intentionally not a guessed short request
timeout. A local process signal still must not be interpreted as remote rollback.

Concrete conveniences delegate to the same Context implementations; mandatory
Core/Action/Gateway/Publisher interfaces do not offer a network fallback to them.
The existing all-production Core selector/method-value regression is retained.
This source inventory is not an alias-sensitive or interprocedural call graph.

Independent cleanup/settlement includes original Action DB settlement (5 s),
PostgreSQL rollback (5 s), owned workspace cleanup (5 s plus traversal budgets),
Codex control/history/stop/close (5–6 s), failed Worker report (6 s), checkpoint
retention (30 s), sandbox cleanup (20 s) and qualification/probe stop budgets.
Publisher Git stop/reap uses the existing 5 s independent process-scope budget.
An unconfirmed reap quarantines its staging instead of deleting under a live
descendant. Publisher witness completion has its own 5 s cancellation-independent,
synchronous lifetime. It never detaches work or runs Publish a second time.

For every filesystem or RWMutex operation, cancellation is cooperative: a
blocking kernel syscall or lock acquisition is not made forcibly interruptible
by this inventory or by a numerical deadline. Tests need to preserve exact
unknown-result and cleanup errors, not merely return quickly.

The existing CI now verifies this exact source digest, ordered path/line/constructor
rows and category counts with `scripts/verify_context_inventory.py`. That gate
prevents a stale inventory from silently staying green; it is not a full call graph.

## Exact direct candidates

`_test.go` files and symlinks are excluded by the retained scanner. Test helper
packages are shown explicitly, not counted as proven production requests.

| Source location | Constructor | Classification |
| --- | --- | --- |
| `cmd/codex-qualifier/main.go:22` | `Background` | Explicitly timed command root |
| `cmd/codex-saved-login-live/main.go:25` | `Background` | Explicitly timed command root |
| `cmd/codex-wif-live/main.go:27` | `Background` | Explicitly timed command root |
| `cmd/control-plane/main.go:57` | `Background` | Signal-owned command/service root |
| `cmd/control-plane/main.go:140` | `Background` | Explicitly timed command root |
| `cmd/eng/artifact_capture.go:63` | `Background` | Explicitly timed command root |
| `cmd/eng/artifact_offline_capture.go:47` | `Background` | Explicitly timed command root |
| `cmd/eng/artifact_retention.go:51` | `Background` | Signal-owned command/service root |
| `cmd/eng/artifact_set.go:82` | `Background` | Explicitly timed command root |
| `cmd/eng/codex_profile.go:121` | `Background` | Explicitly timed command root |
| `cmd/eng/distribution_install.go:63` | `Background` | Signal-owned command/service root |
| `cmd/eng/execution_readback.go:52` | `Background` | Explicitly timed command root |
| `cmd/eng/human_takeover.go:99` | `Background` | Explicitly timed command root |
| `cmd/eng/import_ci.go:54` | `Background` | Explicitly timed command root |
| `cmd/eng/import_engineering.go:33` | `Background` | Explicitly timed command root |
| `cmd/eng/import_engineering.go:77` | `Background` | Explicitly timed command root |
| `cmd/eng/import_engineering.go:113` | `Background` | Explicitly timed command root |
| `cmd/eng/import_engineering.go:162` | `Background` | Explicitly timed command root |
| `cmd/eng/import_engineering.go:198` | `Background` | Explicitly timed command root |
| `cmd/eng/import_engineering.go:257` | `Background` | Explicitly timed command root |
| `cmd/eng/installation_readback.go:49` | `Background` | Signal-owned command/service root |
| `cmd/eng/installation_switch.go:56` | `Background` | Signal-owned command/service root |
| `cmd/eng/pilot_preflight.go:486` | `Background` | Explicitly timed command root |
| `cmd/eng/process_scope.go:13` | `Background` | Command with downstream budget |
| `cmd/eng/production_live_observe.go:102` | `Background` | Signal-owned command/service root |
| `cmd/eng/production_publisher_health.go:42` | `Background` | Signal-owned command/service root |
| `cmd/eng/production_status.go:78` | `Background` | Signal-owned command/service root |
| `cmd/eng/relay_qualification_kit.go:40` | `Background` | Explicitly timed command root |
| `cmd/eng/relay_runtime_handoff.go:34` | `Background` | Explicitly timed command root |
| `cmd/eng/remote.go:41` | `Background` | Command with downstream budget |
| `cmd/eng/run_continue.go:69` | `Background` | Explicitly timed command root |
| `cmd/eng/run_control.go:107` | `Background` | Explicitly timed command root |
| `cmd/eng/source_checkpoint.go:26` | `Background` | Explicitly timed command root |
| `cmd/eng/work_intake.go:298` | `Background` | Explicitly timed command root |
| `cmd/publisher-service/main.go:73` | `Background` | Signal-owned command/service root |
| `cmd/publisher-service/main.go:86` | `Background` | Explicitly timed command root |
| `cmd/sandbox-guard/build_outputs.go:27` | `Background` | Explicitly timed command root |
| `cmd/sandbox-guard/main.go:33` | `Background` | Explicitly timed command root |
| `cmd/worker/main.go:85` | `Background` | Signal-owned command/service root |
| `internal/action/service.go:126` | `WithoutCancel` | Independent bounded cleanup/settlement |
| `internal/action/service.go:299` | `Background` | Concrete convenience wrapper |
| `internal/action/service.go:430` | `Background` | Concrete convenience wrapper |
| `internal/action/service.go:433` | `Background` | Concrete convenience wrapper |
| `internal/action/service.go:436` | `Background` | Concrete convenience wrapper |
| `internal/action/service.go:439` | `Background` | Concrete convenience wrapper |
| `internal/githubpublish/git_scope.go:59` | `WithoutCancel` | Independent bounded cleanup/settlement |
| `internal/githubpublish/service.go:187` | `WithoutCancel` | Independent bounded cleanup/settlement |
| `internal/processscope/probe.go:32` | `Background` | Independent bounded cleanup/settlement |
| `internal/runtime/codexapp/controls.go:122` | `WithoutCancel` | Independent bounded cleanup/settlement |
| `internal/runtime/codexapp/engineering.go:196` | `WithoutCancel` | Independent bounded cleanup/settlement |
| `internal/runtime/codexapp/engineering_exit.go:22` | `WithoutCancel` | Independent bounded cleanup/settlement |
| `internal/runtime/codexapp/engineering_exit.go:34` | `WithoutCancel` | Independent bounded cleanup/settlement |
| `internal/runtime/codexapp/qualification_isolation.go:87` | `Background` | Independent bounded cleanup/settlement |
| `internal/sandbox/engine.go:160` | `Background` | Independent bounded cleanup/settlement |
| `internal/sandbox/testutil/image.go:33` | `Background` | Test support helper |
| `internal/sandbox/testutil/image.go:82` | `Background` | Test support helper |
| `internal/store/memory.go:393` | `Background` | Concrete convenience wrapper |
| `internal/store/memory.go:547` | `Background` | Concrete convenience wrapper |
| `internal/store/memory.go:703` | `Background` | Concrete convenience wrapper |
| `internal/store/memory.go:843` | `Background` | Concrete convenience wrapper |
| `internal/store/memory.go:845` | `Background` | Concrete convenience wrapper |
| `internal/store/memory.go:848` | `Background` | Concrete convenience wrapper |
| `internal/store/memory.go:850` | `Background` | Concrete convenience wrapper |
| `internal/store/memory.go:1102` | `Background` | Concrete convenience wrapper |
| `internal/store/memory.go:1105` | `Background` | Concrete convenience wrapper |
| `internal/store/memory.go:1108` | `Background` | Concrete convenience wrapper |
| `internal/store/memory.go:1114` | `Background` | Concrete convenience wrapper |
| `internal/store/memory.go:1117` | `Background` | Concrete convenience wrapper |
| `internal/store/memory.go:1124` | `Background` | Concrete convenience wrapper |
| `internal/store/memory.go:1127` | `Background` | Concrete convenience wrapper |
| `internal/store/memory.go:1130` | `Background` | Concrete convenience wrapper |
| `internal/store/memory.go:1133` | `Background` | Concrete convenience wrapper |
| `internal/store/memory.go:1136` | `Background` | Concrete convenience wrapper |
| `internal/store/memory.go:1139` | `Background` | Concrete convenience wrapper |
| `internal/store/memory.go:1142` | `Background` | Concrete convenience wrapper |
| `internal/store/memory.go:1145` | `Background` | Concrete convenience wrapper |
| `internal/store/memory.go:1148` | `Background` | Concrete convenience wrapper |
| `internal/store/memory.go:1151` | `Background` | Concrete convenience wrapper |
| `internal/store/memory.go:1154` | `Background` | Concrete convenience wrapper |
| `internal/store/memory.go:1157` | `Background` | Concrete convenience wrapper |
| `internal/store/memory.go:1160` | `Background` | Concrete convenience wrapper |
| `internal/store/memory.go:1163` | `Background` | Concrete convenience wrapper |
| `internal/store/memory.go:1166` | `Background` | Concrete convenience wrapper |
| `internal/store/memory.go:1169` | `Background` | Concrete convenience wrapper |
| `internal/store/memory.go:1172` | `Background` | Concrete convenience wrapper |
| `internal/store/memory.go:1175` | `Background` | Concrete convenience wrapper |
| `internal/store/memory.go:1178` | `Background` | Concrete convenience wrapper |
| `internal/store/memory.go:1181` | `Background` | Concrete convenience wrapper |
| `internal/store/memory.go:1184` | `Background` | Concrete convenience wrapper |
| `internal/store/memory.go:1187` | `Background` | Concrete convenience wrapper |
| `internal/store/memory.go:1190` | `Background` | Concrete convenience wrapper |
| `internal/store/postgres/action_store.go:102` | `Background` | Concrete convenience wrapper |
| `internal/store/postgres/action_store.go:141` | `Background` | Concrete convenience wrapper |
| `internal/store/postgres/action_store.go:149` | `Background` | Concrete convenience wrapper |
| `internal/store/postgres/action_store.go:181` | `Background` | Concrete convenience wrapper |
| `internal/store/postgres/core_store.go:70` | `Background` | Concrete convenience wrapper |
| `internal/store/postgres/core_store.go:105` | `Background` | Concrete convenience wrapper |
| `internal/store/postgres/core_store.go:130` | `Background` | Concrete convenience wrapper |
| `internal/store/postgres/core_store.go:175` | `Background` | Concrete convenience wrapper |
| `internal/store/postgres/core_store.go:296` | `Background` | Concrete convenience wrapper |
| `internal/store/postgres/core_store.go:310` | `Background` | Concrete convenience wrapper |
| `internal/store/postgres/core_store.go:319` | `Background` | Concrete convenience wrapper |
| `internal/store/postgres/core_store.go:340` | `Background` | Concrete convenience wrapper |
| `internal/store/postgres/core_store.go:367` | `Background` | Concrete convenience wrapper |
| `internal/store/postgres/core_store.go:551` | `Background` | Concrete convenience wrapper |
| `internal/store/postgres/core_store.go:581` | `Background` | Concrete convenience wrapper |
| `internal/store/postgres/core_store.go:603` | `Background` | Concrete convenience wrapper |
| `internal/store/postgres/core_store.go:623` | `Background` | Concrete convenience wrapper |
| `internal/store/postgres/core_store.go:687` | `Background` | Concrete convenience wrapper |
| `internal/store/postgres/core_store.go:778` | `Background` | Concrete convenience wrapper |
| `internal/store/postgres/core_store.go:804` | `Background` | Concrete convenience wrapper |
| `internal/store/postgres/core_store.go:887` | `Background` | Concrete convenience wrapper |
| `internal/store/postgres/core_store.go:929` | `Background` | Concrete convenience wrapper |
| `internal/store/postgres/core_store.go:950` | `Background` | Concrete convenience wrapper |
| `internal/store/postgres/core_store.go:985` | `Background` | Concrete convenience wrapper |
| `internal/store/postgres/core_store.go:1005` | `Background` | Concrete convenience wrapper |
| `internal/store/postgres/core_store.go:1057` | `Background` | Concrete convenience wrapper |
| `internal/store/postgres/core_store.go:1077` | `Background` | Concrete convenience wrapper |
| `internal/store/postgres/core_store.go:1112` | `Background` | Concrete convenience wrapper |
| `internal/store/postgres/core_store.go:1136` | `Background` | Concrete convenience wrapper |
| `internal/store/postgres/core_store.go:1242` | `Background` | Concrete convenience wrapper |
| `internal/store/postgres/core_store.go:1262` | `Background` | Concrete convenience wrapper |
| `internal/store/postgres/core_store.go:1342` | `Background` | Concrete convenience wrapper |
| `internal/store/postgres/outbox_store.go:182` | `Background` | Independent bounded cleanup/settlement |
| `internal/store/postgres/store.go:68` | `Background` | Concrete convenience wrapper |
| `internal/store/postgres/store.go:90` | `Background` | Concrete convenience wrapper |
| `internal/store/postgres/store.go:137` | `Background` | Concrete convenience wrapper |
| `internal/testsupport/processscope.go:39` | `Background` | Test support helper |
| `internal/workeragent/codex_checkpoint.go:37` | `Background` | Independent bounded cleanup/settlement |
| `internal/workeragent/failure_settlement.go:27` | `WithoutCancel` | Independent bounded cleanup/settlement |
| `internal/workspace/cleanup.go:23` | `WithoutCancel` | Independent bounded cleanup/settlement |
| `internal/workspace/cleanup.go:151` | `WithoutCancel` | Independent bounded cleanup/settlement |
