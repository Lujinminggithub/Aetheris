import unittest


class BootstrapTests(unittest.TestCase):
    def test_package_exposes_version(self):
        import aetheris

        self.assertEqual(aetheris.__version__, "0.4.15")


if __name__ == "__main__":
    unittest.main()
