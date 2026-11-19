"""Linux real-Git black-box tests, never invoke a model."""
import json
import os
import signal
import time
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

ENG = Path(__file__).resolve().parent.parent / "eng"


class EngV0(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)
        self.state = self.root / "state"
        self.cfg = self.root / "cfg" / "runner.json"
        self.repo = self.root / "repo"
        self.repo.mkdir()
        self.env = dict(os.environ, ENG_HOME=str(self.state), ENG_RUNNER_CONFIG=str(self.cfg))
        self.git(self.repo, "init", "-b", "master")
        (self.repo / "README.md").write_text("before\n")
        self.git(self.repo, "add", "README.md")
        self.git(self.repo, "-c", "user.name=fixture", "-c", "user.email=fixture@invalid", "commit", "-m", "initial")
        self.baseline = self.git(self.repo, "rev-parse", "HEAD").stdout.strip()
        self.runner = self.root / "runner.py"
        self.runner.write_text("""import pathlib, sys, time\ntask = sys.stdin.read()\nif task.startswith('sleep:'):\n    time.sleep(int(task.split(':',1)[1]))\nelif task.startswith('fail:'):\n    pathlib.Path('result.txt').write_text('preserved failure\\n')\n    sys.exit(7)\nelse:\n    pathlib.Path('result.txt').write_text(task + '\\n')\n""")

    @staticmethod
    def git(cwd, *args):
        result = subprocess.run(["git", "-C", str(cwd), *args], text=True, capture_output=True)
        if result.returncode:
            raise AssertionError(f"git {args} failed {result.stderr}")
        return result

    def eng(self, *args, cwd=None, timeout=30):
        return subprocess.run([sys.executable, str(ENG), *args], cwd=cwd or self.repo,
                              text=True, capture_output=True, env=self.env, timeout=timeout)

    def use_fixture(self):
        response = self.eng("use", "--", sys.executable, str(self.runner))
        self.assertEqual(response.returncode, 0, response.stderr)
        self.assertEqual(json.loads(response.stdout)["model_calls"], 0)

    def result(self, response):
        return json.loads(response.stdout)

    def test_no_implicit_runner_and_no_model_call(self):
        response = self.eng("run", "hi")
        self.assertNotEqual(response.returncode, 0)
        self.assertIn("no private approved runner", response.stderr)
        self.assertFalse((self.state / "runs").exists())
        self.assertFalse(self.cfg.exists())

    def test_one_command_from_master_and_checks(self):
        self.use_fixture()
        response = self.eng("run", "after", "--check", "grep -qx after result.txt")
        self.assertEqual(response.returncode, 0, response.stderr)
        status = self.result(response)
        self.assertEqual(status["state"], "locally_verified")
        self.assertNotEqual(status["baseline"], status["result_commit"])
        self.assertEqual(status["checks"][0]["exit_code"], 0)
        self.assertEqual(status["checks"][0]["source_commit"], status["result_commit"])
        self.assertTrue(Path(status["worktree"]).joinpath("result.txt").exists())
        self.assertEqual(self.git(self.repo, "rev-parse", "HEAD").stdout.strip(), self.baseline)
        self.assertEqual(self.git(self.repo, "status", "--porcelain").stdout, "")
        self.assertTrue(Path(status["worktree"]).parent.joinpath("diff.patch").is_file())
        st = self.eng("status", status["id"])
        self.assertEqual(json.loads(st.stdout)["state"], "locally_verified")

    def test_failure_preserves_changes_and_no_retry(self):
        self.use_fixture()
        response = self.eng("run", "fail:one", "--check", "grep -qx ok result.txt")
        self.assertEqual(response.returncode, 1)
        status = self.result(response)
        self.assertEqual(status["state"], "failed")
        self.assertEqual(status["runtime_exit_code"], 7)
        self.assertEqual(Path(status["worktree"]).joinpath("result.txt").read_text(), "preserved failure\n")
        self.assertNotEqual(status["result_commit"], self.baseline)
        self.assertEqual(len(list((self.state / "runs").iterdir())), 1)

    def test_no_checks_stays_review_required(self):
        self.use_fixture()
        response = self.eng("run", "result")
        self.assertEqual(response.returncode, 2, response.stderr)
        self.assertEqual(self.result(response)["state"], "needs_review")
        self.assertFalse(self.result(response)["pr_published"])

    def test_checks_must_not_change_source(self):
        self.use_fixture()
        response = self.eng("run", "content", "--check", "python3 -c 'open(\"result.txt\", \"a\").write(\"changed\")'")
        self.assertEqual(response.returncode, 2, response.stderr)
        self.assertEqual(self.result(response)["state"], "needs_review")

    def test_dry_run_not_invoke_model_and_not_create_workspace(self):
        self.use_fixture()
        response = self.eng("run", "x", "--dry-run", "--check", "false")
        self.assertEqual(response.returncode, 0, response.stderr)
        status = self.result(response)
        self.assertEqual(status["ready_for_local_attempt"], True)
        self.assertTrue(status["checks_not_executed"])
        self.assertFalse((self.state / "runs").exists())

    def test_timeout_marks_unknown_with_no_replay(self):
        self.use_fixture()
        response = self.eng("run", "sleep:30", "--timeout", "1", timeout=12)
        self.assertEqual(response.returncode, 3, response.stderr)
        status = self.result(response)
        self.assertEqual(status["state"], "unknown")
        self.assertNotIn("result_commit", status)
        self.assertEqual(len(list((self.state / "runs").iterdir())), 1)
        self.assertIn("unknown", status["reason"])

    def test_file_task_and_oversized_inline(self):
        self.use_fixture()
        task_file = self.root / "task.md"
        task_file.write_text("hello\n")
        response = self.eng("run", "--task", str(task_file), "--check", "grep -qx hello result.txt")
        self.assertEqual(response.returncode, 0, response.stderr)
        self.assertEqual(self.result(response)["state"], "locally_verified")
        invalid = self.eng("run", "x" * (17 * 1024))
        self.assertNotEqual(invalid.returncode, 0)
        self.assertIn("16 KiB", invalid.stderr)

    def test_runner_arg_placeholder_and_missing_config(self):
        # Command-line argument delivery is explicit, never interpolated by a shell.
        argv_runner = self.root / "argv_runner.py"
        argv_runner.write_text("import pathlib,sys\npathlib.Path('result.txt').write_text(sys.argv[1])\n")
        result = self.eng("use", "--", sys.executable, str(argv_runner), "{prompt}")
        self.assertEqual(result.returncode, 0, result.stderr)
        success = self.eng("run", "quoted ' spaces")
        self.assertEqual(success.returncode, 2, success.stderr)
        status = self.result(success)
        self.assertEqual(status["state"], "needs_review")
        self.assertEqual(Path(status["worktree"]).joinpath("result.txt").read_text(), "quoted ' spaces")


    def test_original_dirty_tree_not_overwritten(self):
        self.use_fixture()
        (self.repo / "README.md").write_text("original uncommitted changes\n")
        response = self.eng("run", "after", "--check", "grep -qx after result.txt")
        self.assertEqual(response.returncode, 0, response.stderr)
        self.assertEqual((self.repo / "README.md").read_text(), "original uncommitted changes\n")
        self.assertEqual(self.git(self.repo, "status", "--porcelain").stdout.strip(), "M README.md")
        self.assertEqual(self.git(self.repo, "rev-parse", "HEAD").stdout.strip(), self.baseline)

    def test_explicit_codex_quota_is_blocked_and_unsafe_filters_refused(self):
        blocked = self.eng("use", "--", "codex", "exec", "{prompt}")
        self.assertNotEqual(blocked.returncode, 0)
        self.assertIn("does not authorize Codex", blocked.stderr)
        self.assertFalse(self.cfg.exists())
        self.use_fixture()
        self.git(self.repo, "config", "--local", "filter.suspicious.clean", "sh -c evil")
        result = self.eng("run", "some change", "--dry-run")
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("executable Git filters", result.stderr)
        self.assertFalse((self.state / "runs").exists())

    def test_interrupt_stops_local_group_without_automatic_retry(self):
        self.use_fixture()
        proc = subprocess.Popen([sys.executable, str(ENG), "run", "sleep:30", "--timeout", "40"],
                                cwd=self.repo, stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                                text=True, env=self.env)
        try:
            deadline = time.monotonic() + 8
            while time.monotonic() < deadline:
                statuses = list((self.state / "runs").glob("*/status.json"))
                if statuses and json.loads(statuses[0].read_text())["state"] == "running":
                    break
                time.sleep(.05)
            else:
                self.fail("fixture did not enter running state")
            proc.send_signal(signal.SIGTERM)
            stdout, stderr = proc.communicate(timeout=10)
            self.assertEqual(proc.returncode, 3, stderr)
            value = json.loads(stdout)
            self.assertEqual(value["state"], "unknown")
            self.assertIn("interrupted", value["reason"])
            self.assertEqual(len(list((self.state / "runs").iterdir())), 1)
        finally:
            if proc.poll() is None:
                proc.kill()
                proc.communicate()


if __name__ == "__main__":
    unittest.main()
