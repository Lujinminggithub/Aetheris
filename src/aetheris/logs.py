from __future__ import annotations

from datetime import datetime, timezone
from typing import Iterable

from .events import AetherisEvent


class DailyLog:
    def build(self, events: Iterable[AetherisEvent]) -> str:
        items = sorted(events, key=lambda event: event.occurred_at)
        date = items[0].occurred_at[:10] if items else datetime.now(timezone.utc).date().isoformat()
        lines = [f"# Aetheris development log - {date}", "", "## Activity", ""]
        for event in items:
            lines.append(f"- {event.event_type} (`{event.event_id}`) from `{event.source}`")
        lines.extend(["", "## Evidence", ""])
        lines.extend(f"- `{event.event_id}`" for event in items)
        return "\n".join(lines) + "\n"

