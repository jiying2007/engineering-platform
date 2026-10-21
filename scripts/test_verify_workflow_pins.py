#!/usr/bin/env python3
import unittest

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

    def test_unsupported_yaml_forms_reject_and_comments_do_not_execute(self):
        for line in ("- {uses: actions/checkout@v7}", "- 'uses': >", '  "uses": *alias'):
            with self.subTest(line=line):
                self.assertTrue(violations(line))
        self.assertEqual(violations("# - uses: actions/checkout@v7\nname: CI"), [])


if __name__ == "__main__":
    unittest.main()
