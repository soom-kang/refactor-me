import { test } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { DEFAULT_CONFIG, loadConfig } from '../src/config.mjs';
import { effortFor, PHASE_EFFORT } from '../src/provider.mjs';

const PHASES = Object.keys(PHASE_EFFORT);

test('the shipped defaults let the per-phase effort table decide', () => {
  // Regression: DEFAULT_CONFIG carried `effort: 'high'` on both agents, and
  // effortFor reads `effort` before PHASE_EFFORT. That made the whole table
  // dead code and ran preflight and characterization at high — the exact spend
  // the table exists to avoid. Assert the resolved value, not the absence of a
  // key, so any future way of reintroducing it fails here too.
  for (const agent of [DEFAULT_CONFIG.agents.codex, DEFAULT_CONFIG.agents.claude]) {
    for (const phase of PHASES) {
      assert.equal(effortFor(phase, agent), PHASE_EFFORT[phase], `${phase}`);
    }
  }
  assert.equal(effortFor('preflight', DEFAULT_CONFIG.agents.claude), 'medium');
  assert.equal(effortFor('characterization', DEFAULT_CONFIG.agents.codex), 'medium');
});

test('the config file we install carries no blanket effort either', () => {
  // config.default.json is copied verbatim to .refactor/config.json on install,
  // so a setting there reaches every user the same way a default would.
  const f = path.join(import.meta.dirname, '..', 'config.default.json');
  const shipped = JSON.parse(fs.readFileSync(f, 'utf8'));
  for (const name of ['codex', 'claude']) {
    assert.equal(shipped.agents[name].effort, undefined, name);
    assert.deepEqual(shipped.agents[name].effort_by_phase ?? {}, {}, name);
  }
});

test('a user who sets effort still overrides the table', () => {
  // The table is our default, not a policy. Someone paying for a cheaper run
  // must still be able to say so, and the merge has to carry it through.
  const dir = fs.mkdtempSync(path.join(process.env.TMPDIR ?? '/tmp', 'rl-cfg-'));
  try {
    fs.mkdirSync(path.join(dir, '.refactor'));
    fs.writeFileSync(
      path.join(dir, '.refactor', 'config.json'),
      JSON.stringify({ agents: { claude: { effort: 'low' }, codex: { effort_by_phase: { audit: 'low' } } } }),
    );
    const cfg = loadConfig(dir);
    assert.equal(effortFor('execute', cfg.agents.claude), 'low');
    assert.equal(effortFor('audit', cfg.agents.codex), 'low');
    // and the phases they did not name keep the table's answer
    assert.equal(effortFor('execute', cfg.agents.codex), 'high');
    // merging a partial agent block must not drop the fields it omitted
    assert.equal(cfg.agents.claude.bin, 'claude');
    assert.equal(cfg.agents.codex.timeout_sec, 1800);
  } finally {
    fs.rmSync(dir, { recursive: true, force: true });
  }
});
