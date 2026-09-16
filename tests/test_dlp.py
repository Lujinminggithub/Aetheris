import hashlib
import unittest


class DLPTests(unittest.TestCase):
    def test_matches_keyword_regex_dictionary_edm_label_and_idm(self):
        from aetheris.dlp import DlpMatcher

        text = "customer@example.com PROJECT-42 secret-tag"
        rules = {
            "keywords": ["PROJECT-42"],
            "regex_patterns": [r"customer@[A-Za-z.]+"],
            "dictionaries": [{"id": "secrets", "terms": ["secret-tag"], "min_matches": 1}],
            "edm_records": [{"id": "customer", "fields": ["customer@example.com"], "min_field_matches": 1}],
            "labels": [{"id": "secret-label", "patterns": ["secret-tag"]}],
            "idm_hashes": [hashlib.sha256(text.encode()).hexdigest()],
        }
        result = DlpMatcher(rules).evaluate(text)
        self.assertEqual({item["kind"] for item in result["matches"]}, {"keyword", "regex", "dictionary", "edm", "label", "idm"})
        self.assertTrue(result["blocked"])
