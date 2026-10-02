"""Release packaging contract checks (no provider or external publication)."""
import importlib.util
from pathlib import Path
import tarfile
import tempfile
import subprocess
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location('prepare', Path(__file__).with_name('prepare-homebrew.py'))
prepare = importlib.util.module_from_spec(spec)
spec.loader.exec_module(prepare)


class PackagingTest(unittest.TestCase):
    def test_candidate_archive_and_formula(self):
        with tempfile.TemporaryDirectory(prefix='refactor source ') as one, tempfile.TemporaryDirectory() as two:
            first = prepare.prepare(one, candidate=True)
            second = prepare.prepare(two, candidate=True)
            self.assertEqual(first['sha256'], second['sha256'])
            self.assertTrue(first['candidate'])
            formula = (Path(one) / 'refactor-me.rb').read_text()
            self.assertIn(first['sha256'], formula)
            self.assertIn(first['commit'], formula)
            self.assertEqual(first['url'], (Path(one) / first['archive']).resolve().as_uri())
            self.assertIn('%20', first['url'])
            self.assertIn(first['url'], formula)
            self.assertNotIn('@VERSION@', formula)
            with tarfile.open(Path(one) / first['archive']) as source:
                names = source.getnames()
                self.assertTrue(any(n.endswith('/tool/go/go.mod') for n in names))
                self.assertFalse(any('/.git/' in n or '/.agents/' in n for n in names))
                self.assertTrue(all(n.startswith('refactor-me-') and '..' not in n.split('/') for n in names))
            with self.assertRaisesRegex(ValueError, 'already exists'):
                prepare.prepare(one, candidate=True)

    def test_release_excludes_ignored_local_source(self):
        with tempfile.TemporaryDirectory() as directory, tempfile.TemporaryDirectory() as out:
            root = Path(directory)
            files = {
                'LICENSE': 'MIT', 'tool/RELEASE_VERSION': '0.10.0-beta.1',
                'tool/CHANGELOG.md': '## 0.10.0-beta.1\n',
                'tool/go/go.mod': 'module example.test/check\n\ngo 1.27\n',
                'tool/go/main.go': 'package main\nfunc main() {}\n',
                '.gitignore': 'tool/go/private.json\n',
                'tool/release/homebrew/refactor-me.rb.in': (prepare.ROOT / 'tool/release/homebrew/refactor-me.rb.in').read_text(),
            }
            for name, body in files.items():
                path = root / name
                path.parent.mkdir(parents=True, exist_ok=True)
                path.write_text(body)
            def run(*args):
                subprocess.run(['git', '-C', str(root), *args], check=True, capture_output=True)
            run('init', '-q')
            run('add', '.')
            run('-c', 'user.name=Fixture', '-c', 'user.email=fixture@example.invalid', '-c', 'commit.gpgsign=false', '-c', 'core.hooksPath=/dev/null', 'commit', '-qm', 'fixture')
            run('tag', 'v0.10.0-beta.1')
            (root / 'tool/go/private.json').write_text('{"local":true}')
            with patch.object(prepare, 'ROOT', root):
                result = prepare.prepare(out)
            self.assertFalse(result['dirty'])
            self.assertFalse(result['candidate'])
            self.assertTrue(result['url'].startswith('https://'))
            with tarfile.open(Path(out) / result['archive']) as source:
                self.assertFalse(any(n.endswith('/private.json') for n in source.getnames()))

    def test_dirty_source_cannot_be_published(self):
        def fake_git(*args):
            return 'a' * 40 if args[0] == 'rev-parse' else ' M README.md'
        with tempfile.TemporaryDirectory() as out, patch.object(prepare, 'git', fake_git):
            with self.assertRaisesRegex(ValueError, 'dirty checkout'):
                prepare.prepare(out)
            self.assertEqual(list(Path(out).iterdir()), [])

    def test_release_tag_must_identify_head(self):
        def fake_git(*args):
            if args[0] == 'status':
                return ''
            return 'a' * 40 if args[1] == 'HEAD' else 'b' * 40
        with tempfile.TemporaryDirectory() as out, patch.object(prepare, 'git', fake_git):
            with self.assertRaisesRegex(ValueError, 'release tag does not identify HEAD'):
                prepare.prepare(out)
            self.assertEqual(list(Path(out).iterdir()), [])


if __name__ == '__main__':
    unittest.main()
