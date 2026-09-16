from __future__ import annotations

from dataclasses import dataclass

from .project_identity import inspect_project_identity
from .project_identity_sync import ProjectIdentityKeys
from .project_registry import ProjectRegistry, ProjectSnapshot


@dataclass(frozen=True)
class ProjectSyncResult:
    registry_revision: int
    projects: list[dict]


class ProjectSync:
    def __init__(self, registry: ProjectRegistry, client):
        self.registry = registry
        self.client = client
        self._last_signature: tuple[int, int] | None = None

    @staticmethod
    def build_batch(snapshot: ProjectSnapshot, keys: ProjectIdentityKeys) -> dict[str, list[dict]]:
        registrations = []
        for project in snapshot.projects:
            if not project.path.is_dir():
                continue
            identity = inspect_project_identity(
                project.path,
                remote_key=keys.remote_key,
                root_key=keys.root_key,
                key_version=keys.key_version,
            )
            registration = identity.to_registration()
            registration.update({
                "display_name": project.display_name or identity.display_name,
                "active": project.state == "active",
                "metadata_revision": snapshot.revision,
            })
            registrations.append(registration)
        return {"projects": registrations}

    def sync(self, keys: ProjectIdentityKeys) -> ProjectSyncResult | None:
        snapshot = self.registry.load()
        signature = (snapshot.revision, keys.key_version)
        if signature == self._last_signature:
            return None
        batch = self.build_batch(snapshot, keys)
        if not batch["projects"]:
            self._last_signature = signature
            return ProjectSyncResult(0, [])
        response = self.client.register_projects(batch)
        if not isinstance(response, dict) or not isinstance(response.get("projects"), list):
            raise ValueError("项目注册响应格式无效")
        try:
            revision = int(response.get("registry_revision", 0))
        except (TypeError, ValueError) as exc:
            raise ValueError("项目注册响应版本无效") from exc
        if revision < 1:
            raise ValueError("项目注册响应版本无效")
        self._last_signature = signature
        return ProjectSyncResult(revision, response["projects"])
