import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]


class ModelProvisioningTests(unittest.TestCase):
    def test_deployment_installs_and_runs_idempotent_model_pull(self):
        script_path = ROOT / "deploy" / "pull-models.sh"
        self.assertTrue(script_path.is_file(), "deploy/pull-models.sh must exist")
        script = script_path.read_text(encoding="utf-8")

        self.assertIn("set -euo pipefail", script)
        self.assertIn("qwen3:4b-instruct", script)
        self.assertIn("embeddinggemma", script)
        self.assertIn("OLLAMA_GENERATION_MODEL", script)
        self.assertIn("OLLAMA_EMBEDDING_MODEL", script)
        self.assertIn("OLLAMA_HOST", script)
        self.assertIn("ollama list", script)
        self.assertIn('ollama pull "$model"', script)
        self.assertIn("/api/tags", script)
        self.assertNotIn("set -x", script)

        installer = (ROOT / "deploy" / "install-server.sh").read_text(encoding="utf-8")
        self.assertIn("pull-models.sh", installer)
        self.assertIn('install -m 0750', installer)

        unit = (ROOT / "deploy" / "systemd" / "ollama.service").read_text(encoding="utf-8")
        self.assertIn("OLLAMA_GENERATION_MODEL=qwen3:4b-instruct", unit)
        self.assertIn("OLLAMA_EMBEDDING_MODEL=embeddinggemma", unit)
        self.assertIn("ExecStartPost=/opt/aetheris/bin/pull-models.sh", unit)


if __name__ == "__main__":
    unittest.main()
