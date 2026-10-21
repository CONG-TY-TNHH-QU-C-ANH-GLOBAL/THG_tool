import importlib.util
import json
from pathlib import Path
import tempfile
import unittest


ROOT = Path(__file__).resolve().parents[2]
SPEC = importlib.util.spec_from_file_location('public_release_preview', ROOT / 'scripts/knowledge/prepare_public_release_preview.py')
MODULE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(MODULE)
REVIEW = ROOT / 'docs/business/company-knowledge/release-review-2026-10-21.json'
PACK = ROOT / 'docs/business/company-knowledge/knowledge.json'


class PublicReleasePreviewTests(unittest.TestCase):
    def test_only_approved_facts_without_internal_guidance(self):
        with tempfile.TemporaryDirectory() as folder:
            destination = Path(folder) / 'preview'
            MODULE.prepare(REVIEW, PACK, destination)
            actual = json.loads((destination / 'knowledge.json').read_text(encoding='utf-8'))
            approved = json.loads(REVIEW.read_text(encoding='utf-8'))['records']
            self.assertEqual(len(actual['records']), 8)
            self.assertEqual({r['id'] for r in actual['records']}, {r['id'] for r in approved})
            for got, want in zip(actual['records'], approved):
                self.assertEqual(got['facts'], want['payload']['facts'])
                self.assertEqual(got['source_ids'], want['payload']['source_ids'])
                self.assertNotIn('guidance', got)
                self.assertNotIn('steps', got)
                self.assertEqual(got['audience'], 'public_reference')

    def test_changed_review_rejected_before_writing(self):
        with tempfile.TemporaryDirectory() as folder:
            review = Path(folder) / 'changed.json'
            review.write_text('{}', encoding='utf-8')
            destination = Path(folder) / 'preview'
            with self.assertRaises(ValueError):
                MODULE.prepare(review, PACK, destination)
            self.assertFalse(destination.exists())


if __name__ == '__main__':
    unittest.main()
