from __future__ import annotations

from collections import defaultdict
from datetime import datetime
from typing import Iterable

from .events import AetherisEvent


class Forge:
    """Build explainable WorkEpisode projections from immutable events."""

    def build(self, events: Iterable[AetherisEvent]) -> list[dict]:
        groups: dict[tuple[str, str], list[AetherisEvent]] = defaultdict(list)
        for event in events:
            groups[(event.project_id, event.session_id)].append(event)
        episodes = []
        for (project_id, session_id), grouped in sorted(groups.items()):
            ordered = sorted(grouped, key=lambda event: event.occurred_at)
            episode_id = f"episode-{project_id}-{session_id}"
            episodes.append(
                {
                    "episode_id": episode_id,
                    "project_id": project_id,
                    "session_id": session_id,
                    "started_at": ordered[0].occurred_at,
                    "ended_at": ordered[-1].occurred_at,
                    "event_ids": [event.event_id for event in ordered],
                    "evidence": [{"event_id": event.event_id, "reason": "same project and session"} for event in ordered],
                    "needs_review": False,
                }
            )
        return episodes

