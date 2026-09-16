from __future__ import annotations

from collections import Counter
from typing import Iterable

from .events import AetherisEvent


class Pulse:
    def summarize(self, events: Iterable[AetherisEvent]) -> dict:
        items = list(events)
        by_source = Counter(event.source for event in items)
        coding = sum(event.event_type in {"process.observed", "git.commit", "git.diff", "ide.activity"} for event in items)
        debugging = sum(event.event_type == "terminal.command" for event in items)
        return {
            "metrics": {
                "total_events": len(items),
                "coding_events": coding,
                "terminal_events": debugging,
                "source_counts": dict(sorted(by_source.items())),
            },
            "explanations": [
                {"metric": "coding_events", "reason": "count of process, Git, and IDE activity events", "value": coding},
                {"metric": "terminal_events", "reason": "count of redacted terminal.command events", "value": debugging},
            ],
            "evidence_event_ids": [event.event_id for event in items],
            "ranking": None,
        }

