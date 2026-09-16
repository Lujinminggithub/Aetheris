import unittest
from pathlib import Path


class RepositorySafetyTests(unittest.TestCase):
    def test_repository_contains_no_forbidden_secret_literals(self):
        forbidden = ["BEGIN " + "RSA PRIVATE KEY", "Authorization: " + "Bearer abc123"]
        chunks = []
        text_suffixes = {".py", ".md", ".toml", ".ps1", ".json", ".yml", ".yaml", ".txt"}
        for path in Path(".").rglob("*"):
            if path.is_file() and path.suffix in text_suffixes and ".superpowers" not in path.parts and "__pycache__" not in path.parts:
                try:
                    chunks.append(path.read_text(encoding="utf-8", errors="ignore"))
                except OSError:
                    continue
        text = "\n".join(chunks)
        self.assertTrue(all(value not in text for value in forbidden))


if __name__ == "__main__":
    unittest.main()
