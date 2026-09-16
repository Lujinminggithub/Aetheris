import json
import os
import tempfile
import unittest
from pathlib import Path


class AISessionAdapterTests(unittest.TestCase):
    @staticmethod
    def _write_codex_session(path: Path, session_id: str, cwd: str, messages: list[str]) -> None:
        lines = [{"type": "session_meta", "payload": {"session_id": session_id, "cwd": cwd}}]
        lines.extend({"type": "response_item", "payload": {"role": "user", "content": [{"type": "input_text", "text": message}]}} for message in messages)
        path.write_text("\n".join(json.dumps(item) for item in lines) + "\n", encoding="utf-8")

    def test_codex_collection_reserves_capacity_for_live_and_backfill_files(self):
        from aetheris.adapters.ai_sessions import AISessionAdapter

        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            for index in range(8):
                path = root / f"session-{index}.jsonl"
                self._write_codex_session(path, f"s-{index}", f"E:/project/{index}", [f"message-{index}-{part}" for part in range(3)])
                os.utime(path, (100 + index, 100 + index))

            records = AISessionAdapter(
                root, tool="codex", max_records=6, live_records=2,
                max_records_per_file=1, live_files=2,
            ).collect()
            sources = {record["provenance"]["source_file"] for record in records}

            self.assertIn("session-7.jsonl", sources)
            self.assertIn("session-6.jsonl", sources)
            self.assertTrue(any(source not in {"session-7.jsonl", "session-6.jsonl"} for source in sources))

    def test_backfill_cursor_resumes_on_a_different_file_after_restart(self):
        from aetheris.adapters.ai_sessions import AISessionAdapter

        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            checkpoint = root / "checkpoint.json"
            for index in range(5):
                self._write_codex_session(root / f"session-{index}.jsonl", f"s-{index}", f"E:/project/{index}", [f"message-{index}"])

            first_adapter = AISessionAdapter(
                root, tool="codex", max_records=2, live_records=0,
                max_records_per_file=1, live_files=0, checkpoint_path=checkpoint,
            )
            first_sources = {record["provenance"]["source_file"] for record in first_adapter.collect()}
            first_adapter.save_checkpoint()
            second_adapter = AISessionAdapter(
                root, tool="codex", max_records=2, live_records=0,
                max_records_per_file=1, live_files=0, checkpoint_path=checkpoint,
            )
            second_sources = {record["provenance"]["source_file"] for record in second_adapter.collect()}

            self.assertEqual(len(first_sources), 2)
            self.assertEqual(len(second_sources), 2)
            self.assertTrue(first_sources.isdisjoint(second_sources))

    def test_collection_progress_reports_file_coverage(self):
        from aetheris.adapters.ai_sessions import AISessionAdapter

        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            for index in range(4):
                self._write_codex_session(root / f"session-{index}.jsonl", f"s-{index}", "E:/project/safe", [f"message-{index}"])
            adapter = AISessionAdapter(
                root, tool="codex", max_records=2, live_records=0,
                max_records_per_file=1, live_files=0,
            )

            adapter.collect()

            self.assertEqual(adapter.progress, {"total_files": 4, "covered_files": 2, "pending_files": 2})

    def test_codex_shell_call_emits_private_tool_event_without_command(self):
        from aetheris.adapters.ai_sessions import AISessionAdapter
        from aetheris.command_privacy import CommandPrivacy

        with tempfile.TemporaryDirectory() as raw:
            path = Path(raw) / "codex.jsonl"
            command = 'Get-ChildItem "D:\\secret" -Recurse -Filter private.dll'
            path.write_text("\n".join([
                json.dumps({"type": "session_meta", "payload": {"id": "session-1", "cwd": "D:/repo"}}),
                json.dumps({"timestamp": "2026-09-07T10:00:00Z", "type": "response_item", "payload": {"type": "function_call", "name": "shell_command", "call_id": "call-1", "arguments": json.dumps({"command": command})}}),
            ]) + "\n", encoding="utf-8")

            records = AISessionAdapter(raw, "codex", command_privacy=CommandPrivacy(b"device-key")).collect()

            tool = next(record for record in records if record["event_type"] == "ai.tool_call")
            self.assertEqual(tool["payload"]["actor_origin"], "ai")
            self.assertEqual(tool["payload"]["session_id"], "session-1")
            self.assertEqual(tool["payload"]["message_id"], "call-1")
            self.assertEqual(tool["payload"]["command_type"], "file.search")
            self.assertNotIn(command, json.dumps(tool, ensure_ascii=False))
            self.assertNotIn("private.dll", json.dumps(tool, ensure_ascii=False))

    def test_codex_exec_custom_tool_call_extracts_static_command_without_exposing_it(self):
        from aetheris.adapters.ai_sessions import AISessionAdapter
        from aetheris.command_privacy import CommandPrivacy

        with tempfile.TemporaryDirectory() as raw:
            path = Path(raw) / "codex.jsonl"
            automation = 'await tools.exec_command({cmd:"Get-Content D:\\\\private\\\\secret.txt"})'
            path.write_text("\n".join([
                json.dumps({"type": "session_meta", "payload": {"id": "session-1", "cwd": "D:/repo"}}),
                json.dumps({"timestamp": "2026-09-07T10:00:00Z", "type": "response_item", "payload": {"type": "custom_tool_call", "name": "exec", "call_id": "call-exec", "input": automation}}),
            ]) + "\n", encoding="utf-8")

            records = AISessionAdapter(raw, "codex", command_privacy=CommandPrivacy(b"device-key")).collect()

            tool = next(record for record in records if record["event_type"] == "ai.tool_call")
            self.assertEqual(tool["payload"]["tool_call_type"], "shell")
            self.assertEqual(tool["payload"]["message_id"], "call-exec")
            self.assertEqual(tool["payload"]["command_hash"], CommandPrivacy(b"device-key").describe('Get-Content D:\\private\\secret.txt').command_hash)
            self.assertNotIn("secret.txt", json.dumps(tool, ensure_ascii=False))
            self.assertNotIn("command", tool["payload"])

    def test_codex_dynamic_exec_falls_back_to_private_automation_event(self):
        from aetheris.adapters.ai_sessions import AISessionAdapter
        from aetheris.command_privacy import CommandPrivacy

        with tempfile.TemporaryDirectory() as raw:
            path = Path(raw) / "codex.jsonl"
            path.write_text(json.dumps({"type": "response_item", "payload": {"type": "custom_tool_call", "name": "exec", "call_id": "dynamic", "input": "await tools.exec_command(buildArgs())"}}) + "\n", encoding="utf-8")

            records = AISessionAdapter(raw, "codex", command_privacy=CommandPrivacy(b"device-key")).collect()

            self.assertEqual(records[0]["payload"]["tool_call_type"], "automation")
            self.assertNotIn("input", records[0]["payload"])

    def test_claude_bash_tool_use_emits_private_tool_event(self):
        from aetheris.adapters.ai_sessions import AISessionAdapter
        from aetheris.command_privacy import CommandPrivacy

        with tempfile.TemporaryDirectory() as raw:
            path = Path(raw) / "claude.jsonl"
            path.write_text(json.dumps({
                "timestamp": "2026-09-07T10:00:00Z", "sessionId": "claude-1", "cwd": "D:/repo",
                "message": {"role": "assistant", "content": [{"type": "tool_use", "id": "tool-1", "name": "Bash", "input": {"command": "git status --short"}}]},
            }) + "\n", encoding="utf-8")

            records = AISessionAdapter(raw, "claude_code", command_privacy=CommandPrivacy(b"device-key")).collect()

            tool = next(record for record in records if record["event_type"] == "ai.tool_call")
            self.assertEqual(tool["payload"]["tool"], "claude_code")
            self.assertEqual(tool["payload"]["command_type"], "vcs")
            self.assertNotIn("git status", json.dumps(tool, ensure_ascii=False))
    def test_parses_allowlisted_jsonl_roles_and_redacts_content(self):
        from aetheris.adapters.ai_sessions import AISessionAdapter

        with tempfile.TemporaryDirectory() as raw:
            session = Path(raw) / "session.jsonl"
            session.write_text(
                json.dumps({"role": "user", "content": "use Bearer abc123"}) + "\n" +
                json.dumps({"role": "assistant", "content": "done"}) + "\n",
                encoding="utf-8",
            )
            records = AISessionAdapter(raw, tool="codex").collect()
            self.assertEqual(len(records), 2)
            self.assertEqual(records[0]["event_type"], "ai.message")
            self.assertNotIn("abc123", str(records))

    def test_parses_claude_history_display_and_codex_response_items(self):
        from aetheris.adapters.ai_sessions import AISessionAdapter

        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            (root / "claude.jsonl").write_text(json.dumps({"display": "fix issue", "timestamp": "2026-09-06", "project": "C:/repo", "sessionId": "s1"}) + "\n", encoding="utf-8")
            (root / "codex.jsonl").write_text(json.dumps({"timestamp": "2026-09-06", "type": "response_item", "payload": {"role": "assistant", "content": [{"type": "output_text", "text": "done"}]}}) + "\n", encoding="utf-8")
            claude = AISessionAdapter(root, tool="claude_code").collect()
            codex = AISessionAdapter(root, tool="codex").collect()
            self.assertEqual(claude[0]["payload"]["session_id"], "s1")
            self.assertEqual(codex[0]["payload"]["role"], "assistant")

    def test_parses_claude_nested_message_format(self):
        from aetheris.adapters.ai_sessions import AISessionAdapter

        with tempfile.TemporaryDirectory() as raw:
            path = Path(raw) / "nested.jsonl"
            path.write_text(json.dumps({"type": "assistant", "message": {"role": "assistant", "content": [{"type": "text", "text": "answer"}]}, "sessionId": "s2", "cwd": "C:/repo"}) + "\n", encoding="utf-8")
            records = AISessionAdapter(raw, tool="claude_code").collect()
            self.assertEqual(records[0]["payload"]["role"], "assistant")
            self.assertEqual(records[0]["payload"]["session_id"], "s2")

    def test_codex_history_uses_session_meta_and_batches_records(self):
        from aetheris.adapters.ai_sessions import AISessionAdapter

        with tempfile.TemporaryDirectory() as raw:
            path = Path(raw) / "rollout.jsonl"
            lines = [
                {"type": "session_meta", "payload": {"session_id": "codex-s1", "cwd": "C:/repo"}},
                {"type": "response_item", "payload": {"role": "user", "content": [{"type": "input_text", "text": "one"}]}},
                {"type": "response_item", "payload": {"role": "assistant", "content": [{"type": "output_text", "text": "two"}]}},
            ]
            path.write_text("\n".join(json.dumps(item) for item in lines) + "\n", encoding="utf-8")
            adapter = AISessionAdapter(raw, tool="codex", max_files=10, max_records=1)
            first = adapter.collect()
            second = adapter.collect()
            self.assertEqual(first[0]["payload"]["session_id"], "codex-s1")
            self.assertEqual(first[0]["payload"]["project"], "C:/repo")
            self.assertEqual(second[0]["payload"]["role"], "assistant")

    def test_codex_history_resumes_from_checkpoint_after_restart(self):
        from aetheris.adapters.ai_sessions import AISessionAdapter

        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            path = root / "rollout.jsonl"
            checkpoint = root / "collector-state.json"
            lines = [
                {"type": "session_meta", "payload": {"session_id": "codex-s1", "cwd": "C:/repo"}},
                {"type": "response_item", "payload": {"role": "user", "content": [{"type": "input_text", "text": "one"}]}},
                {"type": "response_item", "payload": {"role": "assistant", "content": [{"type": "output_text", "text": "two"}]}},
            ]
            path.write_text("\n".join(json.dumps(item) for item in lines) + "\n", encoding="utf-8")

            first_adapter = AISessionAdapter(root, tool="codex", max_records=1, checkpoint_path=checkpoint)
            first = first_adapter.collect()
            first_adapter.save_checkpoint()
            second = AISessionAdapter(root, tool="codex", max_records=1, checkpoint_path=checkpoint).collect()

            self.assertEqual(first[0]["payload"]["content"], "one")
            self.assertEqual(second[0]["payload"]["content"], "two")
            self.assertEqual(second[0]["payload"]["session_id"], "codex-s1")

    def test_identical_history_messages_have_distinct_source_positions(self):
        from aetheris.adapters.ai_sessions import AISessionAdapter

        with tempfile.TemporaryDirectory() as raw:
            path = Path(raw) / "rollout.jsonl"
            line = json.dumps({"role": "user", "content": "repeat"})
            path.write_text(line + "\n" + line + "\n", encoding="utf-8")

            records = AISessionAdapter(raw, tool="codex").collect()

            self.assertEqual(len(records), 2)
            self.assertNotEqual(records[0]["provenance"]["source_offset"], records[1]["provenance"]["source_offset"])


if __name__ == "__main__":
    unittest.main()
