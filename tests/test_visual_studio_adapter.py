import tempfile
import unittest
from pathlib import Path


class VisualStudioAdapterTests(unittest.TestCase):
    def test_collects_solution_activity_without_source_contents(self):
        from aetheris.adapters.visual_studio import VisualStudioAdapter

        with tempfile.TemporaryDirectory() as raw:
            solution = Path(raw) / "Demo.sln"
            solution.write_text("Microsoft Visual Studio Solution File, Format Version 12.00\n", encoding="utf-8")
            records = VisualStudioAdapter(solution).collect()
            self.assertEqual(records[0]["event_type"], "ide.activity")
            self.assertEqual(records[0]["payload"]["tool"], "visual_studio")
            self.assertNotIn("ProjectSection", str(records))


if __name__ == "__main__":
    unittest.main()
