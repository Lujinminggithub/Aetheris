import unittest


class OCRStitchingTests(unittest.TestCase):
    def test_stitcher_combines_frames_in_memory(self):
        from aetheris.adapters.ocr import ScrollStitcher

        frames = [(100, 40), (100, 30), (100, 20)]
        result = ScrollStitcher.combine_sizes(frames)
        self.assertEqual(result, (100, 90))

    def test_missing_ocr_engine_returns_review_state_without_text(self):
        from aetheris.adapters.ocr import OcrCapture

        result = OcrCapture(engine=None).extract(object())
        self.assertEqual(result["state"], "blocked")
        self.assertNotIn("text", result)


if __name__ == "__main__":
    unittest.main()
