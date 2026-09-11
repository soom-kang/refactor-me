import { test } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { VERSION } from '../src/version.mjs';
import { newState, newProviderRing } from '../src/state.mjs';

const root = path.join(import.meta.dirname, '..');

test('the version is a semver triple with an optional numbered prerelease', () => {
  const pattern = /^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-(alpha|beta|rc)\.(0|[1-9]\d*))?$/;
  assert.match(VERSION, pattern);
  for (const value of ['0.1.0', '0.8.8-beta.1', '1.0.0-rc.2']) assert.match(value, pattern);
  for (const value of ['v0.8.8-beta.1', '0.8', '0.8.8-beta', '0.8.8-beta.01', '00.8.8']) {
    assert.doesNotMatch(value, pattern);
  }
});

test('the changelog has a section for the current version', () => {
  // Bumping a release means editing two files. Nothing can merge them — one is
  // a constant, the other is prose — but the drift between them can be made a
  // test failure, and this is it.
  const md = fs.readFileSync(path.join(root, 'CHANGELOG.md'), 'utf8');
  assert.ok(
    new RegExp(`^## ${VERSION.replace(/\./g, '\\.')}\\b`, 'm').test(md),
    `CHANGELOG.md has no "## ${VERSION}" section`,
  );
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
