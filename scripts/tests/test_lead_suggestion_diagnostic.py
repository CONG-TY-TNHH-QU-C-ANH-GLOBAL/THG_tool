"""Privacy and read-only regression tests for the SSH diagnostic's Python block."""
import contextlib
import hashlib
import io
import os
from pathlib import Path
import sqlite3
import tempfile
import textwrap
import unittest
from unittest.mock import patch


WORKFLOW = Path(__file__).resolve().parents[2] / '.github/workflows/lead-suggestion-diagnostic.yml'
source = WORKFLOW.read_text(encoding='utf-8')
start = source.index("python3 - \"$org_id\" <<'PY'", source.index('DIAGNOSTIC_LOOKBACK,DIAGNOSTIC_POST_FBID,DIAGNOSTIC_REVISION python3'))
block = source[start:].split("\n", 1)[1].split('\n            PY', 1)[0]
probe = {'__name__': 'probe_under_test'}
exec(compile(textwrap.dedent(block), str(WORKFLOW), 'exec'), probe)


class DiagnosticTests(unittest.TestCase):
    def test_invalid_inputs_stop_before_subprocess(self):
        cases = [('999999', '', ''), ('20', '1;DROP TABLE leads', ''),
                 ('20', '', 'not-a-commit'), ('20', '0', ''), ('20', '1' * 26, '')]
        for window, post, revision in cases:
            with self.subTest(window=window, post=post), patch.dict(os.environ, {
                'DIAGNOSTIC_LOOKBACK': window, 'DIAGNOSTIC_POST_FBID': post,
                'DIAGNOSTIC_REVISION': revision,
            }), patch('subprocess.check_output') as command:
                with self.assertRaises(ValueError):
                    probe['main']()
                command.assert_not_called()

    def test_fixed_summary_never_echoes_raw_logs(self):
        journal = '\n'.join([
            '[LeadSuggestion] org_id=7 lead_id=1 gate=attempted analysis=no_commercial_need wording=not_applicable reply=false note="PRIVATE-AUTHOR SECRET-TOKEN"',
            '[LeadSuggestion] org_id=70 lead_id=1 gate=attempted reply=true note="OTHER-ORG"',
            'irrelevant org_id=7 reply=true SECRET-TOKEN',
        ])
        lines = probe['scoped_lines'](journal, 7)
        self.assertEqual(len(lines), 1)
        out = io.StringIO()
        with contextlib.redirect_stdout(out):
            probe['summarize'](lines, 'target')
        rendered = out.getvalue()
        self.assertIn('target_suggestion_events=1', rendered)
        self.assertIn('target_analysis_noncommercial=1', rendered)
        self.assertIn('target_with_reply=0', rendered)
        self.assertNotIn('SECRET', rendered)
        self.assertNotIn('PRIVATE', rendered)
        self.assertNotIn('OTHER-ORG', rendered)
        for line in rendered.splitlines():
            self.assertRegex(line, r'^target_[a-z_]+=\d+$')

    def test_target_selection_is_scoped_and_read_only(self):
        with tempfile.TemporaryDirectory() as folder:
            db_path = Path(folder) / 'fixture.db'
            with contextlib.closing(sqlite3.connect(db_path)) as db:
                db.execute('CREATE TABLE leads (id INTEGER, org_id INTEGER, post_fbid TEXT, source_url TEXT)')
                db.executemany('INSERT INTO leads VALUES (?,?,?,?)', [
                    (1, 7, '123', ''), (2, 8, '123', ''),
                    (3, 7, '', 'https://www.facebook.com/groups/111/posts/123/'),
                    (4, 7, '', 'https://www.facebook.com/groups/222/permalink/123'),
                    (5, 7, '1234', 'https://www.facebook.com/groups/111/posts/1234/'),
                ])
                db.commit()
            before = hashlib.sha256(db_path.read_bytes()).digest()
            self.assertEqual(probe['target_ids'](db_path, 7, '123'), {1, 3, 4})
            self.assertEqual(before, hashlib.sha256(db_path.read_bytes()).digest())
            self.assertFalse((Path(folder) / 'missing.db').exists())
            with self.assertRaises(sqlite3.OperationalError):
                probe['target_ids'](Path(folder) / 'missing.db', 7, '123')
            self.assertFalse((Path(folder) / 'missing.db').exists())

    def test_allowed_windows_and_revision(self):
        for window in ('20', '1440', '2880'):
            probe['validate_inputs'](window, '123456789', 'a' * 40)


if __name__ == '__main__':
    unittest.main()
