import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest
from unittest.mock import patch
import verify_blocker_metadata as gate

REPO="jiying2007/engineering-platform"
SNAPSHOT={"version":1,"internal_readiness":"BLOCKED","internal_blockers":[228,229],"external_qualification_gates":[105],"deferred_credential_gates":[103,106]}

def document(snapshot=None):
    return "# Implementation Status\n\n"+gate.START+json.dumps(SNAPSHOT if snapshot is None else snapshot)+gate.END+"\n"

class BlockerMetadataTests(unittest.TestCase):
    def test_nine_keywords_three_reference_forms_case_and_colon(self):
        words="close closes closed fix fixes fixed resolve resolves resolved".split()
        refs=["#228",REPO+"#228","https://github.com/"+REPO+"/issues/228"]
        for word in words:
            for ref in refs:
                for prefix in (word+" ",word.upper()+": ","does not "+word+" "):
                    with self.subTest(prefix=prefix,ref=ref),self.assertRaises(ValueError):
                        gate.verify(document(),REPO,[("candidate",prefix+ref)])

    def test_preserves_open_notation_and_other_repositories(self):
        for text in ("Internal blockers [228,229] remain OPEN", "Refs #228; no completion claim", "fix(publisher): bound original source", "Fixes other/repo#228", "Fixes #999", "Fixes https://github.com/other/repo/issues/228", "The fixed-width reader preserves #228"):
            result=gate.verify(document(),REPO,[("candidate",text)])
            self.assertEqual(result["status"],"DECLARED_BLOCKER_METADATA_CHECKED")
            self.assertFalse(result["issue_changes_performed"])
            self.assertFalse(result["readiness_promoted"])

    def test_no_negation_markdown_or_list_bypass(self):
        for text in ("does not close #228/#229", "Never fixes: #228", "```\nResolves #228\n```", "Fixes `#228`", "fixes [#228](https://github.com/"+REPO+"/issues/228)", "Closes #100, #228", "Fixes #100 and #228", "Fixes:\n#228"):
            with self.subTest(text=text),self.assertRaises(ValueError):
                gate.verify(document(),REPO,[("candidate",text)])
        for number in (103,105,106,229):
            with self.assertRaises(ValueError):
                gate.verify(document(),REPO,[("candidate",f"resolves #{number}")])

    def test_readiness_is_authoritative_and_strict(self):
        valid=dict(SNAPSHOT,internal_readiness="CANDIDATE_ONLY",internal_blockers=[])
        self.assertEqual(gate.unresolved_issues(document(valid)),{103,105,106})
        bad=[]
        for field,value in (("version",True),("version",2),("internal_readiness","READY"),("internal_blockers",[]),("internal_blockers",[229,228]),("internal_blockers",[True,229]),("internal_blockers",[228,228]),("external_qualification_gates",[228]),("external_qualification_gates",[]),("deferred_credential_gates",None)):
            bad.append(document(dict(SNAPSHOT,**{field:value})))
        bad += [document()+document(),"# Implementation Status\n",document().replace(gate.END,""),document().replace('"version": 1','"version": 1, "version": 1'),document(dict(SNAPSHOT,extra=True)),document().replace('"version": 1','"version": NaN')]
        for text in bad:
            with self.subTest(text=text[:90]),self.assertRaises(ValueError):
                gate.verify(text,REPO,[("candidate","safe")])

    def test_pr_push_and_proposed_final_merge_are_all_checked(self):
        events=[{"repository":{"full_name":REPO},"pull_request":{"title":"safe","body":"Closes #228"}},
                {"repository":{"full_name":REPO},"head_commit":{"message":"safe"},"commits":[{"message":"does not close #228"}]}]
        for event in events:
            with self.assertRaises(ValueError):
                gate.verify(document(),REPO,gate.event_messages(event,REPO))
        safe={"repository":{"full_name":REPO},"pull_request":{"title":"safe","body":None}}
        messages=gate.event_messages(safe,REPO)
        gate.verify(document(),REPO,messages)
        with self.assertRaises(ValueError):
            gate.verify(document(),REPO,messages+[("final-squash","does not close #228/#229")])

    def test_wrong_event_missing_fields_and_bounds_reject(self):
        for event in (None,{}, {"repository":{"full_name":False},"pull_request":{"title":"safe","body":None}}, {"repository":{"full_name":"other/repo"},"pull_request":{"title":"safe","body":None}},
                      {"repository":{"full_name":REPO},"pull_request":{"body":"safe"}},
                      {"repository":{"full_name":REPO},"head_commit":False},
                      {"repository":{"full_name":REPO},"commits":"not-a-list"},
                      {"repository":{"full_name":REPO},"commits":[{}]*(gate.MAX_MESSAGES+1)}):
            with self.assertRaises(ValueError):
                gate.event_messages(event,REPO)
        for text in ("x"*(gate.MAX_MESSAGE_BYTES+1),None):
            with self.assertRaises(ValueError):gate.closing_references(text,REPO)
        with self.assertRaises(ValueError):gate.verify(document(),REPO,[])

    def test_strict_numbers_and_aggregate_budgets(self):
        for text in ('{"value":NaN}', '{"value":Infinity}', '{"value":1e9999}', '{"value":0,"value":1}'):
            with self.subTest(json=text), self.assertRaises(ValueError):
                gate.strict_json(text)
        with patch.object(gate,"MAX_TOTAL_MESSAGE_BYTES",7):
            with self.assertRaises(ValueError):
                gate.verify(document(),REPO,[("one","four"),("two","more")])
            event={"repository":{"full_name":REPO},"pull_request":{"title":"four","body":"more"}}
            with self.assertRaises(ValueError):
                gate.event_messages(event,REPO)
        with patch.object(gate,"MAX_FINDINGS",1),self.assertRaises(ValueError):
            gate.verify(document(),REPO,[("candidate","Fixes #228 and #229")])
        with self.assertRaises(ValueError):
            gate.verify(document(),REPO,[("untrusted label text!","safe")])
        with self.assertRaises(ValueError):
            gate.closing_references("safe",None)

    def test_error_does_not_echo_private_message(self):
        secret="NEVER_ECHO_PRIVATE_MESSAGE_SENTINEL"
        with self.assertRaises(ValueError) as raised:
            gate.verify(document(),REPO,[("head_commit",secret+"\nfixes #228")])
        self.assertNotIn(secret,str(raised.exception))
        self.assertIn('"line": 2',str(raised.exception))

    def test_actual_cli_and_nonregular_no_writer_inputs(self):
        with tempfile.TemporaryDirectory() as tmp:
            root=Path(tmp);status=root/"status.md";message=root/"merge.txt"
            status.write_text(document());message.write_text("Internal blockers [228,229] remain OPEN")
            command=[sys.executable,str(Path(gate.__file__).resolve()),"--status",str(status),"--repository",REPO,"--message-file",str(message)]
            proc=subprocess.run(command,capture_output=True,timeout=5)
            self.assertEqual(proc.returncode,0,proc.stderr)
            self.assertFalse(json.loads(proc.stdout)["issue_changes_performed"])
            message.write_text("does not close #228")
            self.assertNotEqual(subprocess.run(command,capture_output=True,timeout=5).returncode,0)
            message.unlink()
            if hasattr(os,"mkfifo"):
                os.mkfifo(message)
                self.assertNotEqual(subprocess.run(command,capture_output=True,timeout=5).returncode,0)
                message.unlink()
            message.symlink_to(status)
            self.assertNotEqual(subprocess.run(command,capture_output=True,timeout=5).returncode,0)
            message.unlink();message.write_bytes(b'\xff')
            self.assertNotEqual(subprocess.run(command,capture_output=True,timeout=5).returncode,0)

if __name__=="__main__":unittest.main()
