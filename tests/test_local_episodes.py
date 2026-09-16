import unittest


class LocalEpisodeTests(unittest.TestCase):
    def test_build_local_episodes_groups_current_device_events(self):
        from aetheris.local_episodes import build_local_episodes

        events = [
            {"event_id": "e1", "device_id": "d1", "project_id": "p1", "session_id": "s1", "event_type": "ai.message", "occurred_at": "2026-09-09T09:00:00Z", "payload": {"role": "user", "content": "修复登录"}},
            {"event_id": "e2", "device_id": "d1", "project_id": "p1", "session_id": "s1", "event_type": "ai.tool_call", "occurred_at": "2026-09-09T09:01:00Z", "payload": {"command_summary": "修改配置"}},
            {"event_id": "other", "device_id": "d2", "project_id": "p1", "session_id": "s1", "event_type": "ai.message", "occurred_at": "2026-09-09T09:01:00Z", "payload": {"role": "user", "content": "其他设备"}},
        ]

        episodes = build_local_episodes(events, device_id="d1")

        self.assertEqual(len(episodes), 1)
        self.assertEqual(episodes[0]["objective"], "修复登录")
        self.assertEqual(episodes[0]["actions"][0]["summary"], "修改配置")
        self.assertEqual(episodes[0]["event_ids"], ["e1", "e2"])

    def test_local_episodes_hide_environment_noise_and_expose_device_and_project_name(self):
        from aetheris.local_episodes import build_local_episodes

        events = [
            {"event_id": "noise", "device_id": "d1", "project_id": "p1", "session_id": "s0", "event_type": "ai.message", "occurred_at": "2026-09-09T08:00:00Z", "payload": {"role": "user", "content": "<environment_context>machine details</environment_context>"}},
            {"event_id": "useful", "device_id": "d1", "project_id": "p1", "session_id": "s1", "event_type": "ai.message", "occurred_at": "2026-09-09T09:00:00Z", "payload": {"role": "user", "content": "修复登录"}},
        ]

        episodes = build_local_episodes(events, device_id="d1", project_names={"p1": "jtagent"})

        self.assertEqual(len(episodes), 1)
        self.assertEqual(episodes[0]["device_id"], "d1")
        self.assertEqual(episodes[0]["project_name"], "jtagent")
        self.assertEqual(episodes[0]["event_count"], 1)

    def test_vscode_behavior_creates_local_episode_without_source_or_character_counts(self):
        from aetheris.local_episodes import build_local_episodes

        events = [{
            "event_id": "save-1", "device_id": "d1", "project_id": "p1", "session_id": "vscode-1",
            "event_type": "ide.file_saved", "occurred_at": "2026-09-14T08:00:00Z",
            "payload": {"relative_path": "src/private.py", "language_id": "python", "inserted_chars": 200},
        }]

        episodes = build_local_episodes(events, device_id="d1", project_names={"p1": "Aetheris"})

        self.assertEqual(episodes[0]["actions"][0]["summary"], "保存 Python 文件")
        self.assertNotIn("private.py", str(episodes))
        self.assertNotIn("200", str(episodes))


if __name__ == "__main__":
    unittest.main()
