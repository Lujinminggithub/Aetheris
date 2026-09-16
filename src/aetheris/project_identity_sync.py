from __future__ import annotations

import base64
import binascii
import secrets
from dataclasses import dataclass
from urllib.error import HTTPError, URLError

from .credentials import ProjectIdentityKeyMaterial, ProjectIdentityKeyStore


@dataclass(frozen=True)
class ProjectIdentityKeys:
    remote_key: bytes
    root_key: bytes
    key_version: int


class ProjectIdentityKeySync:
    def __init__(
        self,
        client,
        tenant_store: ProjectIdentityKeyStore,
        device_store: ProjectIdentityKeyStore,
    ):
        self.client = client
        self.tenant_store = tenant_store
        self.device_store = device_store

    def sync(self) -> ProjectIdentityKeys:
        try:
            tenant = self._from_response(self.client.project_identity_key())
            self.tenant_store.write(tenant)
        except (HTTPError, URLError, OSError):
            tenant = self.tenant_store.read()
        device = self._load_or_create_device_key()
        return ProjectIdentityKeys(tenant.key, device.key, tenant.version)

    @staticmethod
    def _from_response(response: object) -> ProjectIdentityKeyMaterial:
        if not isinstance(response, dict):
            raise ValueError("项目身份密钥响应格式无效")
        try:
            key = base64.b64decode(str(response.get("key", "")), validate=True)
            version = int(response.get("version", 0))
        except (ValueError, TypeError, binascii.Error) as exc:
            raise ValueError("项目身份密钥响应格式无效") from exc
        if len(key) < 32 or version < 1:
            raise ValueError("项目身份密钥响应格式无效")
        return ProjectIdentityKeyMaterial(key, version)

    def _load_or_create_device_key(self) -> ProjectIdentityKeyMaterial:
        try:
            return self.device_store.read()
        except (OSError, ValueError):
            material = ProjectIdentityKeyMaterial(secrets.token_bytes(32), 1)
            self.device_store.write(material)
            return material
