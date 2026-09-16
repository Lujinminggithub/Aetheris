from __future__ import annotations

import re
from typing import Any


class Redactor:
    _bearer = re.compile(r"\bBearer\s+[A-Za-z0-9._~+/=-]+", re.IGNORECASE)
    _api_key = re.compile(r"\b(?:api[_-]?key|token|secret)\s*[:=]\s*['\"]?[^\s,'\"]+", re.IGNORECASE)
    _private_key = re.compile(r"-----BEGIN [^-]+ PRIVATE KEY-----.*?-----END [^-]+ PRIVATE KEY-----", re.DOTALL)
    _email = re.compile(r"\b[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}\b")
    _home_path = re.compile(r"(?i)(?:[A-Z]:\\Users\\[^\\/\s]+|/home/[^/\s]+)")
    _control = re.compile(r"[\x00-\x08\x0b\x0c\x0e-\x1f\x7f]+")

    def redact(self, value: Any) -> tuple[Any, dict[str, Any]]:
        counts: dict[str, int] = {}

        def mark(rule: str) -> None:
            counts[rule] = counts.get(rule, 0) + 1

        def redact_string(text: str) -> str:
            def replace_control(_: re.Match[str]) -> str:
                mark("control_character")
                return "[REDACTED:control]"

            text = self._control.sub(replace_control, text)

            def replace_private(match: re.Match[str]) -> str:
                mark("private_key")
                return "[REDACTED:private_key]"

            text = self._private_key.sub(replace_private, text)

            def replace_bearer(match: re.Match[str]) -> str:
                mark("token")
                return "[REDACTED:token]"

            text = self._bearer.sub(replace_bearer, text)

            def replace_assignment(match: re.Match[str]) -> str:
                key = match.group(1).lower().replace("-", "_")
                mark("token" if key in {"api_key", "apikey", "token", "secret"} else "password")
                return f"{match.group(1)}=[REDACTED:{'token' if key in {'api_key', 'apikey', 'token', 'secret'} else 'password'}]"

            text = re.sub(
                r"\b(api[_-]?key|token|secret|password)\s*[:=]\s*['\"]?([^\s,'\"]+)",
                replace_assignment,
                text,
                flags=re.IGNORECASE,
            )

            def replace_email(match: re.Match[str]) -> str:
                mark("email")
                return "[REDACTED:email]"

            text = self._email.sub(replace_email, text)

            def replace_home(match: re.Match[str]) -> str:
                mark("home_path")
                return "[REDACTED:home_path]"

            return self._home_path.sub(replace_home, text)

        def walk(item: Any) -> Any:
            if isinstance(item, dict):
                result = {}
                for key, child in item.items():
                    normalized = str(key).casefold().replace("-", "_")
                    if normalized in {"token", "api_key", "apikey", "secret"} and child:
                        mark("token")
                        result[key] = "[REDACTED:token]"
                    elif normalized in {"password", "passwd"} and child:
                        mark("password")
                        result[key] = "[REDACTED:password]"
                    else:
                        result[key] = walk(child)
                return result
            if isinstance(item, list):
                return [walk(child) for child in item]
            if isinstance(item, str):
                return redact_string(item)
            return item

        redacted = walk(value)
        return redacted, {
            "rules": [{"rule_id": rule, "count": count} for rule, count in sorted(counts.items())],
            "replacement_count": sum(counts.values()),
        }
