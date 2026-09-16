import unittest


class IdentityTests(unittest.TestCase):
    def test_subject_is_stable_across_machines_and_device_is_not(self):
        from aetheris.identity import device_id, subject_id

        self.assertEqual(subject_id("DOMAIN\\Alice"), subject_id(" domain\\alice "))
        self.assertNotEqual(device_id("DOMAIN\\Alice", "machine-a"), device_id("DOMAIN\\Alice", "machine-b"))


if __name__ == "__main__":
    unittest.main()

