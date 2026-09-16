from __future__ import annotations

import hashlib
import json
from typing import Iterable

from .events import AetherisEvent
from .forge import Forge


class DatasetExporter:
    def export(self, events: Iterable[AetherisEvent]) -> dict:
        items = list(events)
        jsonl = "".join(event.to_json() + "\n" for event in items)
        episodes = Forge().build(items)
        return {
            "manifest": {
                "event_ids": [event.event_id for event in items],
                "episode_ids": [episode["episode_id"] for episode in episodes],
                "rights": {"dataset_export": "required"},
                "redaction": "device-redacted",
                "sha256": hashlib.sha256(jsonl.encode("utf-8")).hexdigest(),
            },
            "jsonl": jsonl,
            "episodes": episodes,
        }

