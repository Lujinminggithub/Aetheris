import json
import sqlite3
import tempfile
import unittest
from pathlib import Path


class RealAdapterAcceptanceTests(unittest.TestCase):
    def test_realistic_claude_jsonl_and_visual_studio_bridge(self):
        from aetheris.adapters.ai_sessions import AISessionAdapter
        from aetheris.adapters.visual_studio_events import VisualStudioEventAdapter

        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            claude = root / "claude"
            claude.mkdir()
            (claude / "session.jsonl").write_text(
                json.dumps({"sessionId": "session-claude", "cwd": "C:/work/demo", "message": {"role": "user", "content": "修复登录超时"}}) + "\n" +
                json.dumps({"sessionId": "session-claude", "cwd": "C:/work/demo", "message": {"role": "assistant", "content": [{"type": "text", "text": "开始检查配置"}]}}) + "\n" +
                json.dumps({"sessionId": "session-claude", "cwd": "C:/work/demo", "message": {"role": "assistant", "content": [{"type": "tool_use", "id": "search-1", "name": "WebSearch", "input": {"query": "Windows authentication timeout"}}]}}) + "\n" +
                json.dumps({"sessionId": "session-claude", "cwd": "C:/work/demo", "message": {"role": "user", "content": [{"type": "tool_result", "tool_use_id": "search-1", "content": "Microsoft documentation result"}]}}) + "\n",
                encoding="utf-8",
            )
            records = AISessionAdapter(claude, "claude_code").collect()
            messages = [item for item in records if item["event_type"] == "ai.message"]
            self.assertEqual([item["payload"]["role"] for item in messages], ["user", "assistant"])
            self.assertIn("C:/work/demo", json.dumps(messages))
            self.assertIn("ai.search_query", [item["event_type"] for item in records])
            self.assertIn("ai.tool_result", [item["event_type"] for item in records])

            bridge = root / "visual-studio.jsonl"
            bridge.write_text("\n".join(json.dumps({"event": event, "solution": "demo.sln", "result": "passed"}) for event in ("build", "debug", "test")) + "\n", encoding="utf-8")
            records = VisualStudioEventAdapter(bridge).collect()
            self.assertEqual([item["event_type"] for item in records], ["ide.build", "ide.debug", "ide.test"])

    def test_realistic_cursor_and_copilot_sqlite_sources(self):
        from aetheris.adapters.copilot import CopilotSessionAdapter
        from aetheris.adapters.cursor import CursorSessionAdapter

        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            cursor = root / "cursor.db"
            con = sqlite3.connect(cursor)
            con.execute("CREATE TABLE cursorDiskKV (key TEXT PRIMARY KEY, value TEXT)")
            con.execute("INSERT INTO cursorDiskKV VALUES (?,?)", ("bubbleId:composer-1:bubble-1", json.dumps({"role": 1, "content": "实现接口", "composerId": "composer-1", "bubbleId": "bubble-1", "modelInfo": {"modelName": "model"}})))
            con.commit(); con.close()
            cursor_records = CursorSessionAdapter(cursor).collect()
            self.assertEqual(cursor_records[0]["payload"]["role"], "user")
            self.assertEqual(cursor_records[0]["payload"]["composer_id"], "composer-1")

            copilot = root / "copilot.db"
            con = sqlite3.connect(copilot)
            con.execute("CREATE TABLE sessions (id TEXT PRIMARY KEY, cwd TEXT, repository TEXT, branch TEXT)")
            con.execute("CREATE TABLE turns (id TEXT PRIMARY KEY, session_id TEXT, turn_index INTEGER, user_message TEXT, assistant_response TEXT, timestamp TEXT)")
            con.execute("INSERT INTO sessions VALUES ('s1','C:/work/demo','demo','main')")
            con.execute("INSERT INTO turns VALUES ('t1','s1',0,'请修复问题','已完成','2026-09-09T09:00:00Z')")
            con.commit(); con.close()
            copilot_records = CopilotSessionAdapter(copilot).collect()
            self.assertEqual(len(copilot_records), 2)
            self.assertEqual(copilot_records[0]["payload"]["tool"], "github_copilot")

    def test_browser_allowlist_rejects_similar_domain(self):
        from aetheris.adapters.browser import BrowserAllowlistAdapter

        adapter = BrowserAllowlistAdapter({"example.com"})
        self.assertIsNotNone(adapter.capture_page("https://docs.example.com/a", "文档"))
        self.assertIsNone(adapter.capture_page("https://notexample.com/a", "不应采集"))


if __name__ == "__main__":
    unittest.main()
