import json
import tempfile
import unittest
from pathlib import Path
from verify_publication_recovery_log import REQUIRED, RESTORE_REQUIRED, verify

class GateTests(unittest.TestCase):
    def test_success_and_negative_events(self):
        package="github.com/jiying2007/engineering-platform/internal/store/postgres"
        good=[{"Action":"pass","Test":name,"Package":package} for name in sorted(REQUIRED) for _ in range(3)]
        good.append({"Action":"pass","Package":package})
        with tempfile.TemporaryDirectory() as tmp:
            path=Path(tmp)/"events.jsonl"
            for events,valid in [(good,True),(good[:-1],False),(good[1:],False),([],False),(good+[{"Action":"skip"}],False),(good+[{"Action":"fail"}],False),([dict(e, Package="different/package") if e.get("Test") else e for e in good],False)]:
                path.write_text("\n".join(json.dumps(e) for e in events))
                if valid: verify(path)
                else:
                    with self.assertRaises(ValueError): verify(path)
    def test_restore_requires_both_drills_without_subtest_skips(self):
        package="github.com/jiying2007/engineering-platform/internal/store/postgres"
        good=[{"Action":"pass","Test":name,"Package":package} for name in sorted(RESTORE_REQUIRED)]
        good.append({"Action":"pass","Package":package})
        with tempfile.TemporaryDirectory() as tmp:
            path=Path(tmp)/"restore.jsonl"
            path.write_text("\n".join(json.dumps(e) for e in good))
            verify(path,1,RESTORE_REQUIRED)
            for bad in (good[1:],good+[{"Action":"skip","Test":"TestPublicationRecoveryRestoreDrill/database"}]):
                path.write_text("\n".join(json.dumps(e) for e in bad))
                with self.assertRaises(ValueError):verify(path,1,RESTORE_REQUIRED)
if __name__=="__main__":unittest.main()
