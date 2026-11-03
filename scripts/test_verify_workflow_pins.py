#!/usr/bin/env python3
import unittest
from pathlib import Path

from verify_workflow_pins import violations


class WorkflowPinsTest(unittest.TestCase):
    def test_immutable_references_and_local_source(self):
        for value in (
            "actions/checkout@" + "a" * 40 + " # v7",
            "'owner/repo/.github/workflows/check.yml@" + "b" * 40 + "'",
            '"owner/repo/action@' + "c" * 40 + '"',
            "docker://alpine@sha256:" + "d" * 64,
            "./.github/actions/local",
        ):
            with self.subTest(value=value):
                self.assertEqual(violations("      - uses: " + value), [])

    def test_tags_branches_short_hashes_and_dynamic_identity_reject(self):
        for value in (
            "actions/checkout@v7", "owner/repo@main", "owner/repo@abcdef0",
            "docker://alpine:latest", "${{ inputs.action }}", "./${{ inputs.action }}",
        ):
            with self.subTest(value=value):
                self.assertTrue(violations("      - uses: " + value))

    def test_ci_artifact_regressions_run_before_heavy_go_work(self):
        # The same independent tests must remain in the final trusted job.
        workflow = Path(".github/workflows/ci.yml").read_text(encoding="utf-8")
        go = workflow.split("\n  go:\n", 1)[1].split("\n  offline:\n", 1)[0]
        evidence = workflow.split("\n  evidence:\n", 1)[1]
        for command in (
            "python3 -B scripts/test_ci_facts_readback.py",
            "python3 -B scripts/test_ci_job_timing.py",
        ):
            with self.subTest(command=command):
                self.assertEqual(go.count(command), 1)
                self.assertEqual(evidence.count(command), 1)
                self.assertLess(go.index(command), go.index("      - uses: actions/setup-go@"))
                self.assertLess(go.index(command), go.index("      - name: Test\\n"))
        self.assertLess(
            go.index("      - name: Fail fast on deterministic CI evidence contracts"),
            go.index("      - name: Download Go modules with bounded retry"),
        )

    def test_unsupported_yaml_forms_reject_and_comments_do_not_execute(self):
        for line in ("- {uses: actions/checkout@v7}", "- 'uses': >", '  "uses": *alias'):
            with self.subTest(line=line):
                self.assertTrue(violations(line))
        self.assertEqual(violations("# - uses: actions/checkout@v7\nname: CI"), [])


if __name__ == "__main__":
    unittest.main()
