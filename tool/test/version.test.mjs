import { test } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { VERSION } from '../src/version.mjs';
import { newState, newProviderRing } from '../src/state.mjs';

const root = path.join(import.meta.dirname, '..');

test('development builds identify themselves as dev', () => {
  const pattern = /^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-(alpha|beta|rc)\.(0|[1-9]\d*))?$/;
  assert.equal(VERSION, 'dev');
  assert.match('0.9.20-beta.1', pattern);
  for (const value of ['v0.9.20-beta.1', '0.9', '0.9.20-beta', '0.9.20-beta.01', '00.9.20']) {
    assert.doesNotMatch(value, pattern);
  }
});

test('the changelog records the next planned release', () => {
  const md = fs.readFileSync(path.join(root, 'CHANGELOG.md'), 'utf8');
  assert.match(md, /0\.9\.20-beta\.1/);
});

test('every run records which build produced it', () => {
  const s = newState({
    id: 'r', repoRoot: '/r', worktree: '/w', baseOid: 'a', baseBranch: 'main',
    branchName: 'b', providers: newProviderRing(['codex']),
  });
  assert.equal(s.toolVersion, VERSION);
  // `schema` is the state file's format, not the tool's — they must stay
  // distinguishable in an old run directory.
  assert.notEqual(s.schema, s.toolVersion);
});

test('the version file is inside src/, where install copies from', () => {
  // install.mjs copies only src/ and bin/. A version file outside them would
  // read correctly here and be missing in every installed copy — the one place
  // it actually has to work.
  assert.ok(fs.existsSync(path.join(root, 'src', 'version.mjs')));
});
