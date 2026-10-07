"""Release packaging contract checks (no provider or external publication)."""
import importlib.util
import json
import os
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
                '.gitignore': 'tool/go/private.json\ntool/go/private.go\n',
                'tool/release/homebrew/refactor-me.rb.in': (prepare.ROOT / 'tool/release/homebrew/refactor-me.rb.in').read_text(),
                'tool/release/build-macos-arm64.sh': (prepare.ROOT / 'tool/release/build-macos-arm64.sh').read_text(),
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
            (root / 'tool/go/private.go').write_text('package main\n// ignored local source\n')
            with patch.object(prepare, 'ROOT', root):
                result = prepare.prepare(out)
            self.assertFalse(result['dirty'])
            self.assertFalse(result['candidate'])
            self.assertTrue(result['url'].startswith('https://'))
            with tarfile.open(Path(out) / result['archive']) as source:
                self.assertFalse(any(n.endswith(('/private.json', '/private.go')) for n in source.getnames()))

            # Probe the actual build cwd and environment without Go or macOS.
            # Stop at compilation; this is not a binary/architecture smoke test.
            bin_dir = Path(out) / 'bin'
            bin_dir.mkdir()
            probe = Path(out) / 'build-input.json'
            stubs = {
                'uname': '#!/bin/sh\nif [ "$1" = -s ]; then echo Darwin; else echo arm64; fi\n',
                'go': '''#!/usr/bin/env python3
import json, os, pathlib, subprocess, sys
cwd = pathlib.Path.cwd()
data = {
    'cwd': str(cwd), 'source': (cwd / 'main.go').read_text(),
    'ignored': [p.name for p in cwd.glob('private.*')],
    'commit': subprocess.check_output(['git', 'rev-parse', 'HEAD'], text=True).strip(),
    'dirty': subprocess.check_output(['git', 'status', '--porcelain'], text=True),
    'env': {k: os.environ.get(k) for k in ['GOENV', 'GOWORK', 'GOFLAGS', 'GOTOOLCHAIN']},
}
pathlib.Path(os.environ['BUILD_INPUT_PROBE']).write_text(json.dumps(data))
sys.exit(73)
''',
            }
            for name, body in stubs.items():
                path = bin_dir / name
                path.write_text(body)
                path.chmod(0o755)
            env = dict(os.environ, PATH=str(bin_dir) + os.pathsep + os.environ['PATH'],
                       BUILD_INPUT_PROBE=str(probe), GOENV='/invalid', GOWORK='/invalid',
                       GOFLAGS='-overlay=/invalid', GOTOOLCHAIN='auto', RELEASE_CANDIDATE='0', RELEASE_TAG='')
            def build(candidate=False):
                return subprocess.run(['bash', str(root / 'tool/release/build-macos-arm64.sh'), str(Path(out) / 'binary')],
                                      env=dict(env, RELEASE_CANDIDATE='1' if candidate else '0'), capture_output=True, text=True)
            run('tag', '-d', 'v0.10.0-beta.1')
            built = build()
            self.assertEqual(built.returncode, 2, built.stderr)
            self.assertIn('release tag v0.10.0-beta.1 is missing', built.stderr)
            self.assertFalse(probe.exists())
            run('tag', 'v0.10.0-beta.1')
            run('-c', 'user.name=Fixture', '-c', 'user.email=fixture@example.invalid', '-c', 'commit.gpgsign=false',
                '-c', 'core.hooksPath=/dev/null', 'commit', '--allow-empty', '-qm', 'next commit')
            built = build()
            self.assertEqual(built.returncode, 2, built.stderr)
            self.assertIn('release tag points to another commit', built.stderr)
            self.assertFalse(probe.exists())
            run('tag', '-f', 'v0.10.0-beta.1')
            built = build()
            self.assertEqual(built.returncode, 73, built.stderr)
            inputs = json.loads(probe.read_text())
            self.assertNotEqual(inputs['cwd'], str((root / 'tool/go').resolve()))
            self.assertEqual(inputs['source'], files['tool/go/main.go'])
            self.assertEqual(inputs['ignored'], [])
            self.assertEqual(inputs['dirty'], '')
            self.assertEqual(inputs['commit'], subprocess.check_output(['git', '-C', str(root), 'rev-parse', 'HEAD'], text=True).strip())
            self.assertEqual(inputs['env'], {'GOENV': 'off', 'GOWORK': 'off', 'GOFLAGS': '', 'GOTOOLCHAIN': 'local'})
            run('tag', '-d', 'v0.10.0-beta.1')
            (root / 'tool/go/main.go').write_text('package main\n// local candidate\n')
            built = build(candidate=True)
            self.assertEqual(built.returncode, 73, built.stderr)
            inputs = json.loads(probe.read_text())
            self.assertEqual(inputs['cwd'], str((root / 'tool/go').resolve()))
            self.assertIn('// local candidate', inputs['source'])
            self.assertTrue(inputs['dirty'])

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
