from __future__ import annotations

import csv
import hashlib
import io
import os
import subprocess
from dataclasses import dataclass
from pathlib import Path
from typing import Literal

from .events import AetherisEvent
from .version import __version__


@dataclass(frozen=True)
class ProcessObservation:
    pid: int
    name: str
    executable: str | None
    command_line: str | None


@dataclass(frozen=True)
class ProcessDecision:
    status: Literal["allowed", "excluded", "pending"]
    reason: str


@dataclass(frozen=True)
class ProjectRef:
    project_id: str
    root: Path
    classification: str = "identified"


class ProcessDiscoverer:
    def discover(self) -> list[ProcessObservation]:
        if os.name != "nt":
            return []
        run_kwargs = {
            "check": False,
            "capture_output": True,
            "text": True,
            "timeout": 10,
        }
        if os.name == "nt":
            run_kwargs["creationflags"] = subprocess.CREATE_NO_WINDOW
        result = subprocess.run(
            ["tasklist", "/FO", "CSV", "/NH"],
            **run_kwargs,
        )
        observations: list[ProcessObservation] = []
        for row in csv.reader(io.StringIO(result.stdout)):
            if len(row) >= 2 and row[1].isdigit():
                observations.append(ProcessObservation(int(row[1]), row[0], None, None))
        return observations


class ConsentRegistry:
    _excluded_fragments = (
        "system", "smss.exe", "csrss.exe", "winlogon.exe", "spoolsv.exe",
        "securityhealth", "defender", "teams", "slack", "wechat", "qq.exe",
    )

    def __init__(self, allowed_names: set[str] | None = None):
        self.allowed_names = {name.casefold() for name in (allowed_names or set())}

    def classify(self, observation: ProcessObservation) -> ProcessDecision:
        name = observation.name.casefold()
        if any(fragment in name for fragment in self._excluded_fragments):
            return ProcessDecision("excluded", "default-excluded-process")
        if name in self.allowed_names:
            return ProcessDecision("allowed", "user-consent-allowlist")
        return ProcessDecision("pending", "unknown-process-requires-confirmation")


class ProjectResolver:
    _markers = (".git", ".svn", "pyproject.toml", "package.json", "go.mod", ".sln")

    def resolve(self, path: str | Path, authorized_roots: list[str | Path]) -> ProjectRef | None:
        candidate = Path(path).expanduser().resolve()
        root = self._find_root(candidate)
        if root is None:
            root = candidate if candidate.is_dir() else None
        if root is None or not self._under_any(root, authorized_roots):
            return None
        digest = hashlib.sha256(str(root).casefold().encode("utf-8")).hexdigest()[:16]
        classification = "identified" if any((root / marker).exists() for marker in self._markers) else "pending"
        return ProjectRef(f"project-{digest}", root, classification)

    def _find_root(self, path: Path) -> Path | None:
        current = path if path.is_dir() else path.parent
        for item in (current, *current.parents):
            if any((item / marker).exists() for marker in self._markers):
                return item
        return None

    @staticmethod
    def _under_any(path: Path, roots: list[str | Path]) -> bool:
        for raw_root in roots:
            try:
                path.relative_to(Path(raw_root).expanduser().resolve())
                return True
            except ValueError:
                continue
        return False


def capture_observation(
    observation: ProcessObservation,
    *,
    project_path: str | Path,
    consent_registry: ConsentRegistry,
    authorized_roots: list[str | Path],
    payload: dict | None = None,
    device_id: str = "device-local",
    tenant_id: str = "local-default",
    subject_id: str = "local-user",
) -> AetherisEvent | None:
    decision = consent_registry.classify(observation)
    if decision.status != "allowed":
        return None
    project = ProjectResolver().resolve(project_path, authorized_roots)
    if project is None:
        return None
    safe_payload = payload or {"pid": observation.pid, "name": observation.name.casefold()}
    return AetherisEvent.create(
        "process.observed",
        tenant_id=tenant_id,
        subject_id=subject_id,
        device_id=device_id,
        project_id=project.project_id,
        session_id=f"session-{observation.pid}",
        source="core.process",
        source_version=__version__,
        payload=safe_payload,
        redaction_report={"rules": [], "replacement_count": 0},
        processing_grants=["server_ingest"],
        provenance={"project_root": str(project.root)},
    )
