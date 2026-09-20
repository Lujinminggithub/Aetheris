import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]


class ModelProvisioningTests(unittest.TestCase):
    def test_deployment_installs_and_runs_idempotent_model_pull(self):
        script_path = ROOT / "deploy" / "pull-models.sh"
        self.assertTrue(script_path.is_file(), "deploy/pull-models.sh must exist")
        script = script_path.read_text(encoding="utf-8")

        self.assertIn("set -euo pipefail", script)
        self.assertIn("qwen3:1.7b", script)
        self.assertIn("embeddinggemma", script)
        self.assertIn("OLLAMA_GENERATION_MODEL", script)
        self.assertIn("OLLAMA_EMBEDDING_MODEL", script)
        self.assertIn("OLLAMA_HOST", script)
        self.assertIn("ollama list", script)
        self.assertIn('ollama pull "$model"', script)
        self.assertIn("/api/tags", script)
        self.assertIn("/api/generate", script)
        self.assertIn("keep_alive", script)
        self.assertIn("30m", script)
        self.assertNotIn("set -x", script)

        installer = (ROOT / "deploy" / "install-server.sh").read_text(encoding="utf-8")
        self.assertIn("pull-models.sh", installer)
        self.assertIn('install -m 0750', installer)

        unit = (ROOT / "deploy" / "systemd" / "ollama.service").read_text(encoding="utf-8")
        self.assertIn("OLLAMA_GENERATION_MODEL=qwen3:1.7b", unit)
        self.assertIn("OLLAMA_EMBEDDING_MODEL=embeddinggemma", unit)
        self.assertIn("ExecStartPost=/opt/aetheris/bin/pull-models.sh", unit)
        self.assertIn("TimeoutStartSec=30min", unit)

    def test_shared_knowledge_deployment_uses_bounded_background_work(self):
        env = (ROOT / "deploy" / ".env.example").read_text(encoding="utf-8")
        self.assertIn("PUBLIC_KNOWLEDGE_COLLECTION=aetheris_public_knowledge_v1", env)
        self.assertIn("PUBLIC_KNOWLEDGE_INTERVAL=300", env)
        self.assertIn("PUBLIC_KNOWLEDGE_BATCH=25", env)
        self.assertIn("PROCESS_KNOWLEDGE_COLLECTION=aetheris_process_knowledge_v1", env)

        unit = (ROOT / "deploy" / "systemd" / "aetheris-server.service").read_text(encoding="utf-8")
        self.assertIn("PUBLIC_KNOWLEDGE_INTERVAL=300", unit)
        self.assertIn("PUBLIC_KNOWLEDGE_BATCH=25", unit)
        self.assertIn("CPUWeight=50", unit)

        for relative in ("scripts/backfill-public-knowledge.sh", "scripts/backfill-public-knowledge.ps1"):
            script = (ROOT / relative).read_text(encoding="utf-8")
            self.assertIn("/api/v1/admin/public-knowledge/jobs", script)
            self.assertIn("aetheris_csrf", script)
            self.assertIn("build_candidates", script)
            self.assertNotIn("set -x", script)
        installer = (ROOT / "deploy" / "install-server.sh").read_text(encoding="utf-8")
        self.assertIn('"$install_root/scripts"', installer)
        self.assertIn("backfill-public-knowledge.sh", installer)


if __name__ == "__main__":
    unittest.main()
