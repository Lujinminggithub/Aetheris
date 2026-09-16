import subprocess
import unittest
from pathlib import Path
from unittest.mock import patch


class SvnAdapterTests(unittest.TestCase):
    def test_collects_revision_and_status_counts_without_repository_url(self):
        from aetheris.adapters.svn import SvnAdapter

        info = '<info><entry revision="12"><repository><root>https://secret.example/repo</root><uuid>repo-uuid</uuid></repository><commit revision="11"><author>dev</author><date>2026-09-06T00:00:00Z</date></commit></entry></info>'
        status = '<status><target path="."><entry path="a"><wc-status item="modified"/></entry><entry path="b"><wc-status item="unversioned"/></entry></target></status>'
        with patch("aetheris.adapters.svn.subprocess.run", side_effect=[subprocess.CompletedProcess([], 0, info, ""), subprocess.CompletedProcess([], 0, status, "")]):
            records = SvnAdapter().collect(Path("repo"))
        payload = records[0]["payload"]
        self.assertEqual(payload["working_copy_revision"], "12")
        self.assertEqual(payload["status_counts"], {"modified": 1, "unversioned": 1})
        self.assertNotIn("repository_url", payload)
        self.assertNotIn("secret.example", str(payload))


if __name__ == "__main__":
    unittest.main()

