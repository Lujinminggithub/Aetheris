import json
import sqlite3
import tempfile
import unittest
from pathlib import Path


class CopilotCursorAdapterTests(unittest.TestCase):
    def test_reads_copilot_chat_turns_from_session_store(self):
        from aetheris.adapters.copilot import CopilotSessionAdapter

        with tempfile.TemporaryDirectory() as raw:
            db = Path(raw) / "session-store.db"
            con = sqlite3.connect(db)
            con.executescript("CREATE TABLE sessions (id TEXT, cwd TEXT, repository TEXT, host_type TEXT, branch TEXT, summary TEXT, agent_name TEXT, agent_description TEXT, created_at TEXT, updated_at TEXT); CREATE TABLE turns (id TEXT, session_id TEXT, turn_index INTEGER, user_message TEXT, assistant_response TEXT, timestamp TEXT);")
            con.execute("INSERT INTO sessions VALUES ('s1','C:/repo','repo','local','main','Fix bug','','','2026-09-06','2026-09-06')")
            con.execute("INSERT INTO turns VALUES ('t1','s1',0,'use Bearer abc123','done','2026-09-06')")
            con.commit(); con.close()
            records = CopilotSessionAdapter(db).collect()
            self.assertEqual(CopilotSessionAdapter(db).tool, "github_copilot")
            self.assertEqual(len(records), 2)
            self.assertEqual(records[0]["event_type"], "ai.message")
            self.assertNotIn("abc123", str(records))

    def test_reads_cursor_bubble_json_from_kv_store(self):
        from aetheris.adapters.cursor import CursorSessionAdapter

        with tempfile.TemporaryDirectory() as raw:
            db = Path(raw) / "state.vscdb"
            con = sqlite3.connect(db)
            con.execute("CREATE TABLE cursorDiskKV (key TEXT, value TEXT)")
            con.execute("INSERT INTO cursorDiskKV VALUES (?, ?)", ("bubbleId:composer:1", json.dumps({"type": "user", "text": "hello", "composerId": "c1", "model": "gpt", "context": [{"path": "app.py"}], "toolCalls": [{"name": "terminal"}]})))
            con.commit(); con.close()
            records = CursorSessionAdapter(db).collect()
            self.assertEqual(CursorSessionAdapter(db).tool, "cursor")
            self.assertEqual(records[0]["event_type"], "ai.message")
            self.assertEqual(records[0]["payload"]["role"], "user")
            self.assertEqual(records[0]["payload"]["composer_id"], "c1")
            self.assertEqual(records[0]["payload"]["tool_calls"], ["terminal"])

    def test_copilot_history_is_emitted_in_bounded_batches(self):
        from aetheris.adapters.copilot import CopilotSessionAdapter

        with tempfile.TemporaryDirectory() as raw:
            db = Path(raw) / "session-store.db"
            con = sqlite3.connect(db)
            con.executescript("CREATE TABLE sessions (id TEXT, cwd TEXT, repository TEXT, branch TEXT); CREATE TABLE turns (id TEXT, session_id TEXT, turn_index INTEGER, user_message TEXT, assistant_response TEXT, timestamp TEXT);")
            con.execute("INSERT INTO sessions VALUES ('s1','C:/repo','repo','main')")
            con.execute("INSERT INTO turns VALUES ('t1','s1',0,'question','answer','2026-09-06')")
            con.commit(); con.close()
            adapter = CopilotSessionAdapter(db, max_records=1)

            first = adapter.collect()
            second = adapter.collect()

            self.assertEqual([item["payload"]["role"] for item in first], ["user"])
            self.assertEqual([item["payload"]["role"] for item in second], ["assistant"])

    def test_cursor_history_is_emitted_in_bounded_batches(self):
        from aetheris.adapters.cursor import CursorSessionAdapter

        with tempfile.TemporaryDirectory() as raw:
            db = Path(raw) / "state.vscdb"
            con = sqlite3.connect(db)
            con.execute("CREATE TABLE cursorDiskKV (key TEXT, value TEXT)")
            con.execute("INSERT INTO cursorDiskKV VALUES (?, ?)", ("bubbleId:c1:1", json.dumps({"type": "user", "text": "one"})))
            con.execute("INSERT INTO cursorDiskKV VALUES (?, ?)", ("bubbleId:c1:2", json.dumps({"type": "assistant", "text": "two"})))
            con.commit(); con.close()
            adapter = CursorSessionAdapter(db, max_records=1)

            first = adapter.collect()
            second = adapter.collect()

            self.assertEqual([item["payload"]["content"] for item in first], ["one"])
            self.assertEqual([item["payload"]["content"] for item in second], ["two"])

    def test_cursor_numeric_bubble_format_maps_roles_and_composer_metadata(self):
        from aetheris.adapters.cursor import CursorSessionAdapter

        with tempfile.TemporaryDirectory() as raw:
            db = Path(raw) / "state.vscdb"
            con = sqlite3.connect(db)
            con.execute("CREATE TABLE cursorDiskKV (key TEXT, value TEXT)")
            con.execute(
                "INSERT INTO cursorDiskKV VALUES (?, ?)",
                (
                    "bubbleId:composer-1:bubble-1",
                    json.dumps({
                        "type": 2,
                        "text": "answer",
                        "modelInfo": {"modelName": "gpt-test"},
                        "toolFormerData": {"name": "read_file", "result": "must-not-be-copied"},
                    }),
                ),
            )
            con.commit(); con.close()

            record = CursorSessionAdapter(db).collect()[0]

            self.assertEqual(record["payload"]["role"], "assistant")
            self.assertEqual(record["payload"]["composer_id"], "composer-1")
            self.assertEqual(record["payload"]["bubble_id"], "bubble-1")
            self.assertEqual(record["payload"]["model"], "gpt-test")
            self.assertEqual(record["payload"]["tool_calls"], ["read_file"])
            self.assertNotIn("must-not-be-copied", str(record))


if __name__ == "__main__":
    unittest.main()
