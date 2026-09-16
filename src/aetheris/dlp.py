from __future__ import annotations

import hashlib
import re


class DlpMatcher:
    def __init__(self, rules: dict | None = None):
        self.rules = rules or {}

    def evaluate(self, text: str, content_hash: str | None = None) -> dict:
        matches = []
        for keyword in self.rules.get("keywords", []):
            count = text.casefold().count(str(keyword).casefold())
            if count:
                matches.append({"kind": "keyword", "rule_id": str(keyword), "count": count})
        for index, pattern in enumerate(self.rules.get("regex_patterns", []), start=1):
            count = len(re.findall(pattern, text, flags=re.IGNORECASE))
            if count:
                matches.append({"kind": "regex", "rule_id": f"regex-{index}", "count": count})
        for dictionary in self.rules.get("dictionaries", []):
            count = sum(text.casefold().count(str(term).casefold()) for term in dictionary.get("terms", []))
            if count >= int(dictionary.get("min_matches", 1)):
                matches.append({"kind": "dictionary", "rule_id": dictionary.get("id", "dictionary"), "count": count})
        for record in self.rules.get("edm_records", []):
            count = sum(str(field).casefold() in text.casefold() for field in record.get("fields", []))
            if count >= int(record.get("min_field_matches", 1)):
                matches.append({"kind": "edm", "rule_id": record.get("id", "edm"), "count": count})
        for label in self.rules.get("labels", []):
            count = sum(str(pattern).casefold() in text.casefold() for pattern in label.get("patterns", []))
            if count:
                matches.append({"kind": "label", "rule_id": label.get("id", "label"), "count": count})
        digest = content_hash or hashlib.sha256(text.encode("utf-8")).hexdigest()
        if any(str(value).casefold() == digest.casefold() for value in self.rules.get("idm_hashes", [])):
            matches.append({"kind": "idm", "rule_id": digest, "count": 1})
        return {"matches": matches, "blocked": bool(matches), "content_hash": digest}

