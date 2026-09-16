from __future__ import annotations

import getpass
import hashlib
import os
import socket
import unicodedata


def normalize_user_identifier(identifier: str) -> str:
    value = unicodedata.normalize("NFKC", identifier).strip().casefold()
    if not value:
        raise ValueError("用户标识不能为空")
    return value


def subject_id(identifier: str) -> str:
    digest = hashlib.sha256(normalize_user_identifier(identifier).encode("utf-8")).hexdigest()[:16]
    return f"subject-{digest}"


def device_id(identifier: str, machine_guid: str) -> str:
    if not machine_guid.strip():
        raise ValueError("机器标识不能为空")
    source = f"{machine_guid.strip().casefold()}\0{normalize_user_identifier(identifier)}"
    return "device-" + hashlib.sha256(source.encode("utf-8")).hexdigest()[:16]


def default_user_identifier() -> str:
    domain = os.environ.get("USERDOMAIN", "").strip()
    username = getpass.getuser().strip()
    return f"{domain}\\{username}" if domain else username


def machine_guid() -> str:
    if os.name == "nt":
        try:
            import winreg
            with winreg.OpenKey(winreg.HKEY_LOCAL_MACHINE, r"SOFTWARE\Microsoft\Cryptography") as key:
                return str(winreg.QueryValueEx(key, "MachineGuid")[0])
        except OSError:
            pass
    return socket.gethostname()

