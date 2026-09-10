import { test } from 'node:test';
import assert from 'node:assert/strict';
import { mergeUsage, newUsage, noteUsage, costOf, fmtTokens, fmtMs, classifyClaude, classifyCodex } from '../src/provider.mjs';
import { newState, newProviderRing } from '../src/state.mjs';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';

const st = () => newState({
  id: 'r', repoRoot: '/r', worktree: '/w', baseOid: 'a', baseBranch: 'main',
  branchName: 'b', providers: newProviderRing(['claude', 'codex']),
});
const u = (i, o, c) => ({ inputTokens: i, outputTokens: o, costUsd: c });

test('an unreported cost stays null instead of collapsing to zero', () => {
  // codex never emits a turn cost. Folding null to 0 would render "$0.00",
  // which a reader would believe. It has to stay unknown.
  const m = mergeUsage(u(10, 1, null), u(20, 2, null));
  assert.equal(m.costUsd, null);
  assert.equal(m.costMissing, 2);
  assert.equal(m.inputTokens, 30);
});

test('a partial cost is kept, and how much is missing is kept with it', () => {
  const m = mergeUsage(u(10, 1, 0.30), u(20, 2, null));
  assert.equal(m.costUsd, 0.30);
  assert.equal(m.costMissing, 1);
  assert.equal(costOf(m).partial, true);
  assert.equal(costOf(m).text, '$0.30');
});

test('costOf never prices an unreported total at zero', () => {
  const { text, partial } = costOf(mergeUsage(u(1, 1, null), u(1, 1, null)));
  assert.equal(text, null);          // the caller must say "미보고", not "$0.00"
  assert.equal(partial, true);
});

test('noteUsage fills the provider ring and the phase bucket at once', () => {
  const s = st();
  noteUsage(s, { provider: 'codex', phase: 'audit', usage: u(100, 20, null), durationMs: 5000 });
  noteUsage(s, { provider: 'codex', phase: 'audit', usage: u(50, 10, null), durationMs: 3000 });
  assert.equal(s.providers.codex.usage.inputTokens, 150);
  assert.equal(s.providers.codex.usage.ms, 8000);
  assert.equal(s.usage.byPhase.audit.calls, 2);
  assert.equal(s.usage.byPhase.audit.outputTokens, 30);
  assert.equal(s.usage.byPhase.audit.costUsd, null);
  assert.equal(s.usage.byPhase.audit.costMissing, 2);
});

test('a failed call still counts toward what the run spent', () => {
  // A thirty-minute timeout spent thirty minutes. The report answers "what did
  // this cost", not "what did it cost to succeed" — but the two stay separable.
  const s = st();
  noteUsage(s, { provider: 'claude', phase: 'execute', usage: u(10, 0, 0.02), durationMs: 1_800_000, ok: false });
  assert.equal(s.usage.byPhase.execute.ms, 1_800_000);
  assert.equal(s.usage.byPhase.execute.failedCalls, 1);
  assert.equal(s.usage.byPhase.execute.calls, 1);
});

test('a repaired call is one call but two processes', () => {
  // callWithRepair merges the two CLI runs into one result carrying
  // processes: 2. `calls` must not follow, because the terminal summary has
  // always printed calls and TUTORIAL documents that number.
  const s = st();
  noteUsage(s, { provider: 'claude', phase: 'review', usage: u(10, 2, 0.1), durationMs: 100, processes: 2 });
  assert.equal(s.usage.byPhase.review.calls, 1);
  assert.equal(s.usage.byPhase.review.processes, 2);
  assert.equal(s.providers.claude.usage.processes, 2);
});

test('a fresh accumulator reports nothing rather than zero dollars', () => {
  assert.equal(newUsage().costUsd, null);
  assert.equal(newUsage().processes, 0);
});

test('claude usage is read off the result envelope, cost included', () => {
  const stdout = JSON.stringify({
    type: 'result', is_error: false, structured_output: { ok: true },
    usage: { input_tokens: 1200, output_tokens: 80 }, total_cost_usd: 0.34,
  });
  const p = classifyClaude({ stdout, stderr: '', killed: false, schemaRequested: false }).parsed;
  assert.deepEqual(p.usage, { inputTokens: 1200, outputTokens: 80, costUsd: 0.34 });
});

test('claude without a reported cost yields null, not zero', () => {
  const stdout = JSON.stringify({
    type: 'result', is_error: false, structured_output: {},
    usage: { input_tokens: 5, output_tokens: 1 },
  });
  const p = classifyClaude({ stdout, stderr: '', killed: false, schemaRequested: false }).parsed;
  assert.equal(p.usage.costUsd, null);
  assert.equal(p.usage.inputTokens, 5);
});

test('token counts read at a glance and durations read as clock time', () => {
  assert.equal(fmtTokens(412), '412');
  assert.equal(fmtTokens(38_200), '38.2k');
  assert.equal(fmtTokens(812_400), '812k');
  assert.equal(fmtMs(8_000), '8s');
  assert.equal(fmtMs(372_000), '6m12s');
});

test('codex reports tokens but never a cost, and that null is the contract', () => {
  // Not a bug and not a parse miss: the codex CLI does not emit a turn cost at
  // all. Everything downstream — mergeUsage, costOf, the report — exists to
  // carry this asymmetry honestly rather than print $0.00.
  const stdout = [
    JSON.stringify({ type: 'thread.started', thread_id: 't1' }),
    JSON.stringify({ type: 'turn.completed', usage: { input_tokens: 900, output_tokens: 40 } }),
  ].join('\n');
  const out = path.join(os.tmpdir(), `rl-usage-${process.pid}.json`);
  fs.writeFileSync(out, '{"ok":true}');
  try {
    const p = classifyCodex({ stdout, stderr: '', exitCode: 0, killed: false, outPath: out, schemaRequested: true }).parsed;
    assert.deepEqual(p.usage, { inputTokens: 900, outputTokens: 40, costUsd: null });
  } finally {
    fs.rmSync(out, { force: true });
  }
});

test('claude input tokens include the cached ones, so both providers mean the same thing', () => {
  // The CLIs disagree: codex's input_tokens is a total with cached_input_tokens
  // as a subset, while claude's excludes the cache entirely. A real deep_check
  // call read 292k tokens and reported input_tokens: 18. Publishing that number
  // as "input tokens" would be worse than publishing nothing.
  const stdout = JSON.stringify({
    type: 'result', is_error: false, structured_output: {},
    usage: { input_tokens: 18, cache_creation_input_tokens: 42_886, cache_read_input_tokens: 249_075, output_tokens: 14_146 },
    total_cost_usd: 0.5488,
  });
  const p = classifyClaude({ stdout, stderr: '', killed: false, schemaRequested: false }).parsed;
  assert.equal(p.usage.inputTokens, 18 + 42_886 + 249_075);
  assert.equal(p.usage.outputTokens, 14_146);
});
