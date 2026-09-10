// What the audit model can actually see. The agents get no shell, so
// collectRepoFacts IS their field of view — a file missing from the inventory is
// a file they will not survey. That makes this the real implementation of
// --target, and these tests check the field of view rather than the prose.
import { test, before, after } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import os from 'node:os';
import { spawnSync } from 'node:child_process';
import { collectRepoFacts } from '../src/prompts.mjs';

const here = import.meta.dirname;
let repo;

const AREAS = { areas: ['.', 'app/web', 'app/api'], commands: [], skipped: [] };

before(() => {
  repo = fs.mkdtempSync(path.join(os.tmpdir(), 'rl-facts-'));
  fs.rmSync(repo, { recursive: true, force: true });
  // --multi lays out root + app/web + app/api, which is the shape a scoped run
  // exists for: several areas in one repository.
  const r = spawnSync('node', [path.join(here, '..', 'fixtures', 'make-fixture.mjs'), repo, '--multi'], { encoding: 'utf8' });
  assert.equal(r.status, 0, r.stderr);
});

after(() => { if (repo) fs.rmSync(repo, { recursive: true, force: true }); });

const inventory = (text) => text.slice(text.indexOf('file inventory (lines, path):'));

test('an unscoped run sees every area', () => {
  const inv = inventory(collectRepoFacts(repo, AREAS));
  assert.ok(inv.includes('app/web/'), 'app/web must be listed');
  assert.ok(inv.includes('app/api/'), 'app/api must be listed');
  assert.ok(!collectRepoFacts(repo, AREAS).includes('scan scope:'),
    'an unscoped run must read exactly as it did before --target existed');
});

test('a scoped run lists only the target', () => {
  const text = collectRepoFacts(repo, { ...AREAS, targets: ['app/web'] });
  const inv = inventory(text);
  assert.ok(inv.includes('app/web/'), 'the target must be listed');
  assert.ok(!inv.includes('app/api/'), 'another area must not reach the inventory');
  assert.ok(!/\n\s*\d+\s+src\//.test(inv), 'the root area must not reach the inventory either');
});

// Dropping the rest silently would teach the model that they do not exist, and
// it would then "prove" a symbol dead without ever grepping the callers.
test('a scoped run says the unlisted files are still reachable', () => {
  const text = collectRepoFacts(repo, { ...AREAS, targets: ['app/web'] });
  assert.match(text, /scan scope: app\/web/);
  assert.match(text, /Glob and Grep/);
  assert.match(text, /tracked source files in scope: \d+/);
});

test('multiple targets are all in scope', () => {
  const inv = inventory(collectRepoFacts(repo, { ...AREAS, targets: ['app/web', 'app/api'] }));
  assert.ok(inv.includes('app/web/'));
  assert.ok(inv.includes('app/api/'));
});

// The "files at or over 500 lines" figure is what steers LARGE_COMPONENT_SPLIT
// candidates; counting out-of-scope files there would invite the model to
// propose exactly the work the operator scoped away.
test('the large-file statistic follows the scope', () => {
  const scoped = collectRepoFacts(repo, { ...AREAS, targets: ['app/api'] });
  const big = /files at or over 500 lines: (\d+|none)/.exec(scoped)[1];
  if (big !== 'none') {
    const named = scoped.slice(scoped.indexOf('files at or over 500 lines:')).split('\n')[0];
    assert.ok(!named.includes('app/web/'), 'a file outside the scope must not be named');
  }
});
