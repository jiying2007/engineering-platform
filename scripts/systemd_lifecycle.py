#!/usr/bin/env python3
"""CI-only installed Publisher/systemd regression; never a deployment command.

Only randomly named transient test units are created. The installed canonical
Publisher's service settings are retained; paths and the test UID/GID differ.
No model, valid upstream publication, production configuration or SQL is used.
"""
import argparse
import configparser
import hashlib
import json
import os
from pathlib import Path
import queue
import re
import shutil
import ssl
import subprocess
import sys
import threading
import time
import urllib.request

PREFIX = re.compile(r"ep-sysd-[0-9]+-[0-9]+-[a-f0-9]{12}\Z")
SHA = re.compile(r"[a-f0-9]{40}\Z")
STOP_POLICY = {"KillMode": "control-group", "KillSignal": "SIGTERM", "SendSIGKILL": "yes"}
RATE_POLICY = {"StartLimitIntervalSec": "60s", "StartLimitBurst": "3"}
ROLES = ("control-plane", "publisher", "worker-admission", "worker-preparation")


def require(ok, message):
    if not ok:
        raise ValueError(message)


def command(argv, *, timeout=35, check=True):
    result = subprocess.run(argv, capture_output=True, text=True, timeout=timeout)
    if check and result.returncode:
        # Do not copy command lines/configuration/logs into public test output.
        raise ValueError("test command failed: " + Path(argv[0]).name + " exit=" + str(result.returncode))
    return result


def parse_unit(raw):
    require(len(raw) <= 16384, "unit exceeds bound")
    p = configparser.ConfigParser(interpolation=None, strict=True, empty_lines_in_values=False)
    p.optionxform = str
    p.read_string(raw)
    require(set(p.sections()) == {"Unit", "Service", "Install"} and not p.defaults(), "unexpected unit sections")
    result = {section: dict(p[section]) for section in p.sections()}
    for section in result.values():
        require(all(value and "\n" not in value for value in section.values()), "empty/multiline property")
    for key, value in RATE_POLICY.items():
        require(result["Unit"].get(key) == value, "restart bound absent or changed: " + key)
    for key, value in {**STOP_POLICY, "Restart": "on-failure", "RestartSec": "5s", "UMask": "0077", "NoNewPrivileges": "true", "ProtectSystem": "strict", "ProtectHome": "true"}.items():
        require(result["Service"].get(key) == value, "service boundary absent or changed: " + key)
    executable = result["Service"].get("ExecStart")
    notify = executable in ("/opt/engineering-platform/bin/control-plane --production", "/opt/engineering-platform/bin/publisher-service")
    require(result["Service"].get("Type") == ("notify" if notify else "simple"), "role startup notification contract drift")
    if notify:
        require(result["Service"].get("NotifyAccess") == "main", "only main process may notify")
    else:
        require("NotifyAccess" not in result["Service"], "worker must not inherit endpoint readiness")
    return result


def publisher_properties(raw, root, uid, gid):
    p = parse_unit(raw)
    require(p["Unit"]["Description"] == "Engineering Platform GitHub Publisher", "wrong service role")
    require(p["Service"]["ExecStart"] == "/opt/engineering-platform/bin/publisher-service", "publisher executable drift")
    require(p["Service"]["User"] == "engineering-publisher" and p["Service"]["Group"] == "engineering-platform", "publisher identity contract drift")
    require(p["Service"]["EnvironmentFile"] == "/etc/engineering-platform/publisher.env", "publisher environment drift")
    require(p["Service"]["ReadOnlyPaths"] == "/var/lib/engineering-platform/preparation/artifacts", "publisher artifact boundary drift")
    # Unit graph intentionally not instantiated: no production-named unit is
    # loaded, started, stopped, enabled or reconfigured by this test.
    require(set(p["Unit"]) == {"Description", "Wants", "After", *RATE_POLICY}, "unreviewed unit graph/property")
    allowed = {"Type", "NotifyAccess", "User", "Group", "EnvironmentFile", "ExecStart", "Restart", "RestartSec", "TimeoutStartSec", "TimeoutStopSec", "UMask", "NoNewPrivileges", "PrivateTmp", "PrivateDevices", "ProtectSystem", "ProtectHome", "ProtectKernelTunables", "ProtectKernelModules", "ProtectKernelLogs", "ProtectControlGroups", "ProtectClock", "ProtectHostname", "RestrictSUIDSGID", "LockPersonality", "RestrictAddressFamilies", "ReadOnlyPaths", "SystemCallArchitectures", *STOP_POLICY}
    require(set(p["Service"]) == allowed, "unreviewed service property")
    properties = {**RATE_POLICY, **p["Service"]}
    del properties["ExecStart"]
    properties.update(User=str(uid), Group=str(gid), EnvironmentFile=str(root / "publisher.env"), ReadOnlyPaths=str(root / "artifacts"))
    return ["--property=" + key + "=" + value for key, value in sorted(properties.items())]


def fixture(root):
    (root / "artifacts").mkdir(mode=0o700)
    def openssl(*args):
        command(["openssl", *args])
    ca, cakey = root / "ca.crt", root / "ca.key"
    openssl("req", "-x509", "-newkey", "ec", "-pkeyopt", "ec_paramgen_curve:P-256", "-nodes", "-days", "1", "-subj", "/CN=ep-ci-only", "-keyout", str(cakey), "-out", str(ca), "-addext", "basicConstraints=critical,CA:TRUE", "-addext", "keyUsage=critical,keyCertSign,cRLSign", "-addext", "subjectKeyIdentifier=hash")
    for name, usage, san in (("server", "serverAuth", "IP:127.0.0.1"), ("client", "clientAuth", "URI:urn:engineering-platform:control:systemd-test")):
        openssl("req", "-new", "-newkey", "ec", "-pkeyopt", "ec_paramgen_curve:P-256", "-nodes", "-subj", "/CN=ep-ci-" + name, "-keyout", str(root / (name + ".key")), "-out", str(root / (name + ".csr")))
        ext = root / (name + ".ext")
        ext.write_text("basicConstraints=critical,CA:FALSE\nkeyUsage=critical,digitalSignature\nextendedKeyUsage=" + usage + "\nsubjectAltName=" + san + "\nauthorityKeyIdentifier=keyid:always\nsubjectKeyIdentifier=hash\n")
        openssl("x509", "-req", "-in", str(root / (name + ".csr")), "-CA", str(ca), "-CAkey", str(cakey), "-set_serial", "11" if name == "server" else "12", "-days", "1", "-extfile", str(ext), "-out", str(root / (name + ".crt")))
    (root / "inert-token").write_text("TEST_ONLY_NOT_AN_UPSTREAM_CREDENTIAL")
    (root / "publisher.json").write_text(json.dumps({"version": 1, "artifact_root": str(root / "artifacts"), "git_executable": str(Path(shutil.which("git")).resolve()), "token_file": str(root / "inert-token"), "targets": [{"repository": "example/systemd-fixture", "base_ref": "main", "branch_prefix": "test/"}]}))
    env = {"PUBLISHER_CONFIG_FILE": root / "publisher.json", "PUBLISHER_TLS_CERT_FILE": root / "server.crt", "PUBLISHER_TLS_KEY_FILE": root / "server.key", "PUBLISHER_CLIENT_CA_FILE": ca, "PUBLISHER_CONTROL_SUBJECT": "urn:engineering-platform:control:systemd-test", "LISTEN_HOST": "127.0.0.1", "PORT": "0"}
    (root / "publisher.env").write_text("".join(str(k) + "=" + str(v) + "\n" for k, v in env.items()))
    tls = ssl.create_default_context(cafile=str(ca))
    tls.load_cert_chain(str(root / "client.crt"), str(root / "client.key"))
    # Bypass any inherited proxy. This client can only target parsed loopback.
    return urllib.request.build_opener(urllib.request.ProxyHandler({}), urllib.request.HTTPSHandler(context=tls))


class Unit:
    def __init__(self, name, properties, argv):
        require(PREFIX.fullmatch(name), "invalid isolated test unit")
        self.name = name + ".service"
        self.endpoints = queue.Queue()
        self.lines = []
        self.output_error = None
        self.p = subprocess.Popen(["sudo", "-n", "systemd-run", "--quiet", "--wait", "--pipe", "--unit=" + self.name, *properties, "--", *argv], stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True)
        self.reader = threading.Thread(target=self._read, daemon=True)
        self.reader.start()

    def _read(self):
        size = 0
        for line in iter(lambda: self.p.stdout.readline(65537), ""):
            size += len(line)
            if size > 65536:
                self.output_error = "test service log exceeds bound"
                break
            self.lines.append(line)
            m = re.search(r"publisher service listening on (127\.0\.0\.1:[1-9][0-9]*);", line)
            if m:
                self.endpoints.put("https://" + m.group(1))

    def endpoint(self):
        end = time.monotonic() + 20
        while time.monotonic() < end:
            require(not self.output_error, "test output bound exceeded")
            try:
                return self.endpoints.get(timeout=0.1)
            except queue.Empty:
                require(self.p.poll() is None, "managed Publisher exited before authenticated probe")
        raise ValueError("managed Publisher startup timed out")

    def ctl(self, *args, check=True):
        return command(["sudo", "-n", "systemctl", *args, self.name], check=check)

    def snapshot(self):
        r = self.ctl("show", "--property=LoadState,ActiveState,SubState,MainPID,NRestarts,Result,ControlGroup,User,Group,KillMode,SendSIGKILL,StartLimitBurst,StartLimitIntervalUSec,RestartUSec,TimeoutStopUSec,Type,NotifyAccess,ActiveEnterTimestampMonotonic,ExecMainStartTimestampMonotonic,Job", check=False)
        value = dict(line.split("=", 1) for line in r.stdout.splitlines() if "=" in line)
        require(r.returncode == 0 or value.get("LoadState") == "not-found", "cannot observe test unit")
        return value

    def finish(self, success):
        rc = self.p.wait(timeout=35)
        self.reader.join(timeout=2)
        require(not self.reader.is_alive() and not self.output_error, "test log reader incomplete")
        require((rc == 0) == success, "systemd-run exit did not match expected outcome")

    def cleanup(self):
        # Restrict cleanup to the exact generated name even on test failure.
        try:
            self.ctl("stop", check=False)
            self.ctl("reset-failed", check=False)
        finally:
            if self.p.poll() is None:
                self.p.kill()
            self.p.wait(timeout=5)
            self.reader.join(timeout=2)
            if self.p.stdout:
                self.p.stdout.close()



def assert_rate_limit(state, attempts, manager_records, unit_name):
    """Require manager-origin limit evidence, not a single Result label.

    systemd v255 retains an earlier service exit-code when can_start rejects
    the next restart (service_enter_dead only overwrites SERVICE_SUCCESS).
    The PID1 journal event separately records that the actual limit was hit.
    """
    require(PREFIX.fullmatch(unit_name.removesuffix(".service")) and unit_name.endswith(".service"), "invalid limit subject")
    require(state.get("LoadState") == "loaded" and state.get("ActiveState") == "failed" and state.get("SubState") == "failed" and state.get("MainPID") == "0", "limited service did not remain stopped")
    require(state.get("Result") in ("exit-code", "start-limit-hit") and state.get("StartLimitBurst") == "3" and state.get("StartLimitIntervalUSec") == "1min", "unexpected limited service result/policy")
    require(attempts == 3, "expected exactly three rejected startup attempts")
    require(isinstance(manager_records, list) and len(manager_records) <= 64, "bounded manager records required")
    require(any(isinstance(row, dict) and row.get("_PID") == "1" and row.get("UNIT") == unit_name and row.get("MESSAGE") == unit_name + ": Start request repeated too quickly." for row in manager_records), "manager did not independently confirm start limit")


def cgroup_empty(group):
    require(re.fullmatch(r"/system.slice/ep-sysd-[0-9]+-[0-9]+-[a-f0-9]{12}\.service", group), "unexpected test cgroup")
    root = Path("/sys/fs/cgroup") / group.lstrip("/")
    if not root.exists():
        return True
    return all(not p.read_text().strip() for p in root.rglob("cgroup.procs"))


def run_suite(distribution, expected, root, prefix):
    require(os.environ.get("EP_SYSTEMD_INTEGRATION") == "1" and os.environ.get("GITHUB_ACTIONS") == "true", "requires explicit ephemeral GitHub CI gate")
    require(os.getuid() != 0 and os.getgid() != 0, "tests and service must use non-root identity")
    require(Path("/run/systemd/system").is_dir() and Path("/run/systemd/private").exists(), "actual systemd manager required; no skip/fallback")
    require(re.fullmatch(r"ep-sysd-[0-9]+-[0-9]+", prefix), "invalid CI invocation prefix")
    require(SHA.fullmatch(expected), "exact externally expected source required")
    require(root.is_absolute() and root == root.resolve() and root.parent == Path("/run") and root.name.startswith(prefix + "-"), "private CI work root required")
    st = root.stat()
    require(st.st_uid == os.getuid() and st.st_mode & 0o777 == 0o700 and not any(root.iterdir()), "empty caller-private root required")
    command(["sudo", "-n", "true"])
    os.umask(0o077)
    copied = root / "distribution"
    shutil.copytree(distribution, copied, symlinks=True)
    installed = root / "installed"
    installer = copied / "eng"
    result = json.loads(command([str(installer), "distribution-install", "--from", str(copied), "--into", str(installed), "--source-commit", expected]).stdout)
    require(result["status"] == "INSTALLED_BYTES_VERIFIED" and not result["services_started"], "installation gate failed")
    shutil.rmtree(copied)
    eng = installed / "bin/eng"
    command([str(eng), "installation-verify", "--dir", str(installed), "--source-commit", expected])
    assets = installed / "templates/systemd"
    for role in ROLES:
        parse_unit((assets / ("engineering-" + role + ".service")).read_text())
    raw = (assets / "engineering-publisher.service").read_text()
    props = publisher_properties(raw, root, os.getuid(), os.getgid())
    opener = fixture(root)
    publisher = str(installed / "bin/publisher-service")
    units = []
    checks = []
    def unit(argv, overrides=None, extra=None):
        selected = dict(item.removeprefix("--property=").split("=", 1) for item in props)
        selected.update(overrides or {})
        selected.update(extra or {})
        u = Unit(prefix + "-" + os.urandom(6).hex(), ["--property=" + key + "=" + value for key, value in sorted(selected.items())], argv)
        units.append(u)
        return u
    def health(endpoint):
        require(re.fullmatch(r"https://127\.0\.0\.1:[1-9][0-9]*", endpoint), "invalid loopback endpoint")
        with opener.open(endpoint + "/healthz", timeout=3) as response:
            value = json.loads(response.read(4096))
            require(response.status == 200 and value == {"service": "engineering-github-publisher", "status": "ok"}, "authenticated identity health mismatch")
    def activated(u):
        deadline = time.monotonic() + 5
        while True:
            state = u.snapshot()
            if state.get("ActiveState") == "active":
                require(state.get("SubState") == "running" and state.get("Type") == "notify" and state.get("NotifyAccess") == "main", "manager readiness attribution mismatch")
                return state
            require(u.p.poll() is None and time.monotonic() < deadline, "manager did not accept main-process readiness")
            time.sleep(0.02)
    try:
        live = unit([publisher])
        health(live.endpoint())
        before = activated(live)
        require(before["User"] == str(os.getuid()) and before["Group"] == str(os.getgid()) and before["KillMode"] == "control-group" and before["SendSIGKILL"] == "yes" and before["StartLimitBurst"] == "3", "manager ignored identity/stop/restart policy")
        require(before["StartLimitIntervalUSec"] == "1min" and before["RestartUSec"] == "5s" and before["TimeoutStopUSec"] == "20s", "manager duration policy drift")
        live.ctl("kill", "--kill-whom=main", "--signal=SIGKILL")
        health(live.endpoint())
        after = activated(live)
        require(after["MainPID"] != before["MainPID"] and int(after["NRestarts"]) == 1, "manager did not restart once with a fresh process")
        live.ctl("stop")
        live.finish(True)
        require(cgroup_empty(after["ControlGroup"]), "stopped Publisher cgroup is not empty")
        checks.append("installed_publisher_mtls_crash_restart_and_explicit_stop")

        rejected = unit([publisher, "--unsupported-ci-only"])
        rejected.finish(False)
        state = rejected.snapshot()
        # Preserve the actual service result; an earlier exit-code may remain.
        # Only PID1's event for this exact random unit proves rate limiting.
        log = command(["sudo", "-n", "journalctl", "--quiet", "--no-pager", "--output=json", "-n", "64", "_PID=1", "UNIT=" + rejected.name]).stdout
        require(len(log) <= 1 << 20, "manager journal response exceeds bound")
        records = [json.loads(line) for line in log.splitlines() if line]
        attempts = sum("publisher-service accepts no command-line arguments" in line for line in rejected.lines)
        assert_rate_limit(state, attempts, records, rejected.name)
        time.sleep(6)  # greater than RestartSec, still inside the 60s limit
        require(rejected.snapshot() == state and sum("publisher-service accepts no command-line arguments" in line for line in rejected.lines) == attempts, "rate-limited service restarted unexpectedly")
        rate_result = {"service_result": state["Result"], "rejected_starts": attempts, "manager_limit_event": True, "observed_no_restart_seconds": 6}
        checks.append("three_rejected_starts_then_manager_confirmed_rate_limit")

        # A clearly separate fault fixture verifies descendant cleanup. It is
        # not a model turn or evidence about Publisher's Git subprocesses.
        child = unit(["/bin/sh", "-c", "trap '' TERM; /usr/bin/setsid /bin/sleep 300 & wait"], {"Type": "simple", "NotifyAccess": "none"})
        end = time.monotonic() + 10
        while True:
            state = child.snapshot()
            group = state.get("ControlGroup", "")
            if group and state.get("MainPID", "0") != "0":
                cg = Path("/sys/fs/cgroup") / group.lstrip("/") / "cgroup.procs"
                if cg.exists() and len(cg.read_text().splitlines()) >= 2:
                    break
            require(time.monotonic() < end and child.p.poll() is None, "descendant fault fixture did not start")
            time.sleep(0.05)
        started = time.monotonic()
        child.ctl("stop", check=False)
        child.finish(False)
        require(time.monotonic() - started >= 19 and cgroup_empty(group), "manager stop did not enforce timeout and reap descendants")
        state = child.snapshot()
        require(state["Result"] == "timeout" and state["MainPID"] == "0", "forced stop must remain a recorded timeout, not success")
        time.sleep(6)  # beyond RestartSec: explicit stop must not replay fixture
        require(cgroup_empty(group) and child.snapshot()["MainPID"] == "0", "explicit stop incorrectly restarted work")
        checks.append("setsid_descendant_forced_stop_no_replay")
        # A non-notifying process must never satisfy the endpoint startup gate.
        silent = unit(["/bin/sleep", "300"], {"Restart": "no", "TimeoutStartSec": "2s", "TimeoutStopSec": "2s"})
        deadline = time.monotonic() + 1
        while True:
            starting = silent.snapshot()
            if starting.get("ActiveState") == "activating" and starting.get("ControlGroup"):
                silent_group = starting["ControlGroup"]
                break
            require(silent.p.poll() is None and time.monotonic() < deadline, "silent notification fixture did not activate")
            time.sleep(0.01)
        silent.finish(False)
        state = silent.snapshot()
        require(state.get("Result") == "timeout" and state.get("MainPID") == "0" and state.get("ActiveEnterTimestampMonotonic") == "0", "missing READY became manager startup success")
        require(cgroup_empty(silent_group), "timed-out startup left processes")
        checks.append("missing_notification_never_becomes_active")

        # A generated dependency transaction tests the same Requires/After
        # semantics used by Worker units, without claiming the full unit graph.
        release = root / "notify-release"
        code = ("import os,socket,time; from pathlib import Path; "
                "p=Path(" + repr(str(release)) + "); "
                "exec(\"while not p.exists(): time.sleep(0.01)\"); "
                "s=socket.socket(socket.AF_UNIX,socket.SOCK_DGRAM); "
                "s.connect(os.environ['NOTIFY_SOCKET'].replace('@','\\0',1) if os.environ['NOTIFY_SOCKET'].startswith('@') else os.environ['NOTIFY_SOCKET']); "
                "s.send(b'READY=1'); s.close(); time.sleep(60)")
        supplier = unit([sys.executable, "-c", code], {"Restart": "no"})
        deadline = time.monotonic() + 5
        while True:
            waiting = supplier.snapshot()
            if waiting.get("SubState") == "start" and waiting.get("MainPID", "0") != "0":
                break
            require(supplier.p.poll() is None and time.monotonic() < deadline, "notification fixture not starting")
            time.sleep(0.02)
        dependent = unit(["/bin/true"], {"Type": "oneshot", "NotifyAccess": "none", "Restart": "no", "RemainAfterExit": "yes"}, {"Requires": supplier.name, "After": supplier.name})
        deadline = time.monotonic() + 5
        while True:
            pending = dependent.snapshot()
            if pending.get("LoadState") == "loaded" and pending.get("Job"):
                require(pending.get("MainPID") == "0" and pending.get("ActiveState") == "inactive", "dependent started before notification")
                break
            require(dependent.p.poll() is None and time.monotonic() < deadline, "dependent transaction missing")
            time.sleep(0.02)
        release.write_text("release explicit test fixture only\n")
        active = activated(supplier)
        deadline = time.monotonic() + 5
        while True:
            completed = dependent.snapshot()
            if completed.get("ActiveState") == "active" and completed.get("SubState") == "exited":
                require(int(completed["ExecMainStartTimestampMonotonic"]) >= int(active["ActiveEnterTimestampMonotonic"]) > 0, "manager released dependency before accepted readiness")
                break
            require(dependent.p.poll() is None and time.monotonic() < deadline, "dependent not released after notification")
            time.sleep(0.02)
        dependent.ctl("stop")
        dependent.finish(True)
        supplier.ctl("stop")
        supplier.finish(True)
        checks.append("requires_after_waits_for_main_ready_datagram")

        command([str(eng), "installation-verify", "--dir", str(installed), "--source-commit", expected])
        return {"version": 1, "source_commit": expected, "status": "TEST_CASES_PASSED", "checks": checks, "rate_limit_observation": rate_result, "template_sha256": hashlib.sha256(raw.encode()).hexdigest(), "manager": command(["systemctl", "--version"]).stdout.splitlines()[0], "distinct_production_users_tested": False, "production_unit_graph_tested": False, "model_turn_executed": False, "upstream_publication_executed": False, "production_qualified": False}
    finally:
        cleanup_errors = []
        for u in reversed(units):
            try:
                u.cleanup()
            except Exception as exc:
                cleanup_errors.append(type(exc).__name__)
        if cleanup_errors:
            raise ValueError("test units require cleanup reconciliation: " + ",".join(cleanup_errors))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--distribution", required=True, type=Path)
    parser.add_argument("--source", required=True)
    parser.add_argument("--work-root", required=True, type=Path)
    parser.add_argument("--prefix", required=True)
    args = parser.parse_args()
    try:
        result = run_suite(args.distribution, args.source, args.work_root, args.prefix)
        print(json.dumps(result, indent=2, sort_keys=True))
        return 0
    except Exception as exc:
        print("systemd regression rejected: " + str(exc), file=sys.stderr)
        return 1


if __name__ == "__main__":
    sys.exit(main())
