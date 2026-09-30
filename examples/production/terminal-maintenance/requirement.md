# Production Terminal Maintenance Qualification

This is the one-time production/unattended terminal acceptance fixture for Engineering Platform.

The authorized change is intentionally narrow:

- change only `examples/production/terminal-maintenance/qualification.txt`;
- exact before bytes: `PRE_LIVE_READY\n`;
- exact after bytes: `TERMINAL_QUALIFIED\n`;
- do not change product behavior, Core authority, provider policy, CI policy, or any other file.

The execution must use the exact selected unattended Provider v3 identity after its independent live qualification, the independent Publisher service, exact PR-head CI, Codex/Git/CI Evidence, Verification, independent human Review, and Closure.

This fixture is RELEASE qualification work. It is not a third Feature or Debug pilot.
