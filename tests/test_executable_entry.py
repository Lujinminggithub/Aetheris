import subprocess
import unittest
from pathlib import Path


class ExecutableEntryTests(unittest.TestCase):
    def test_top_level_entrypoint_supports_version_without_relative_import_error(self):
        result = subprocess.run(
            ["python", "scripts/tray_entry.py", "--version"],
            cwd=Path(__file__).resolve().parents[1],
            capture_output=True,
            text=True,
            check=False,
        )
        self.assertEqual(result.returncode, 0)
        self.assertEqual(result.stdout.strip(), "0.4.16")
        self.assertNotIn("attempted relative import", result.stderr)


if __name__ == "__main__":
    unittest.main()
