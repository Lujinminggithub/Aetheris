import os
import subprocess
import unittest
from unittest.mock import patch


class HiddenProcessTests(unittest.TestCase):
    @unittest.skipUnless(os.name == "nt", "Windows process flags only apply on Windows")
    def test_tasklist_and_git_processes_use_no_window_flag(self):
        from aetheris.core import ProcessDiscoverer
        from aetheris.adapters.git import GitAdapter

        with patch("aetheris.core.subprocess.run", return_value=subprocess.CompletedProcess([], 0, '"Code.exe","1"\n', "")) as tasklist:
            ProcessDiscoverer().discover()
            self.assertTrue(tasklist.call_args.kwargs["creationflags"] & subprocess.CREATE_NO_WINDOW)
        with patch("aetheris.adapters.git.subprocess.run", return_value=subprocess.CompletedProcess([], 0, "", "")) as git_run:
            try:
                GitAdapter()._run(os.getcwd(), ["git", "--version"])
            except Exception:
                pass
            self.assertTrue(git_run.call_args.kwargs["creationflags"] & subprocess.CREATE_NO_WINDOW)


if __name__ == "__main__":
    unittest.main()
