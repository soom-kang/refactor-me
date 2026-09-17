import { test } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { auditPrompt } from '../src/prompts.mjs';
import { auditDirFor } from '../src/state.mjs';

const base = {
  provider: 'codex', nonce: 'n', repoFacts: 'file inventory (lines, path):\n   10  a.ts',
  seen: { done: [], skipped: [] }, violated: [], targets: [],
};

// The re-audit that produced no candidates was handed two claims it should not
// have been: that commits it could see were already in the tree, and that seven
// files nobody had touched were off limits.
test('a re-audit with no commits does not claim the tree contains any', () => {
  const out = auditPrompt({ ...base, incremental: true, cycle: 2, committed: 0 });
  assert.doesNotMatch(out, /ALREADY CONTAINS/);
  assert.match(out, /No earlier cycle of this run landed a commit/);
  assert.match(out, /nothing in the code has changed since you last saw it/,
    'the model must know re-reading is not wasted, only re-proposing is');
});

test('a re-audit after commits still says the tree contains them', () => {
  const out = auditPrompt({ ...base, incremental: true, cycle: 3, committed: 2 });
  assert.match(out, /ALREADY CONTAINS/);
  assert.match(out, /landed 2 commit\(s\)/);
});

test('a first-cycle audit carries no history block at all', () => {
  const out = auditPrompt({ ...base, incremental: false, cycle: 1, committed: 0 });
  assert.doesNotMatch(out, /ALREADY CONTAINS/);
  assert.doesNotMatch(out, /No earlier cycle of this run landed a commit/);
  assert.doesNotMatch(out, /PRIOR_WORK/);
});

test('only real violations reach the forbidden-ground line', () => {
  const clean = auditPrompt({ ...base, incremental: true, cycle: 2, committed: 0, violated: [] });
  assert.doesNotMatch(clean, /must not touch/,
    'a candidate filtered during ranking was never attempted, so it forbids nothing');

  const dirty = auditPrompt({ ...base, incremental: true, cycle: 2, committed: 0, violated: ['pnpm-lock.yaml'] });
  assert.match(dirty, /must not touch: pnpm-lock\.yaml/);
});

test('each cycle gets its own audit artifact directory', () => {
  const run = fs.mkdtempSync(path.join(os.tmpdir(), 'rl-auditdir-'));
  try {
    const one = auditDirFor(run, 1);
    const two = auditDirFor(run, 2);
    assert.notEqual(one, two, 'a re-audit used to overwrite the first cycle transcript');
    assert.equal(path.basename(one), '01');
    assert.equal(path.basename(two), '02');
    assert.ok(fs.existsSync(path.join(one, 'provider')));
    // The documented `audits/*.json` glob must still match only the JSON.
    fs.writeFileSync(path.join(run, 'audits', '01.json'), '{}');
    assert.ok(fs.statSync(path.join(run, 'audits', '01.json')).isFile());
    assert.ok(fs.statSync(one).isDirectory());
  } finally { fs.rmSync(run, { recursive: true, force: true }); }
});
