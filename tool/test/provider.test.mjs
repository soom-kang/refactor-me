import { test } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import {
  claudeArgs, codexArgs, classifyClaude, classifyCodex,
  lastJsonObject, stripFences, readyProviders, pickProvider, pickReviewer, noteFailure,
} from '../src/provider.mjs';
import { newProviderRing } from '../src/state.mjs';
import { callProvider, modeFor, WRITE_PHASES } from '../src/provider.mjs';

// ---------------------------------------------------------------- argv

test('claude read mode has no write tool and no Bash', () => {
  const a = claudeArgs({ mode: 'read', schema: { type: 'object' } });
  assert.equal(a[a.indexOf('--tools') + 1], 'Skill,Read,Glob,Grep', 'the tool set itself is the read-only gate');
  assert.equal(a[a.indexOf('--permission-mode') + 1], 'plan');
  const denied = a.slice(a.indexOf('--disallowed-tools') + 1, a.indexOf('--setting-sources'));
  for (const t of ['Edit', 'Write', 'Bash', 'WebFetch']) assert.ok(denied.includes(t), `${t} must be denied`);
});

test('claude write mode grants Edit/Write but still no Bash', () => {
  const a = claudeArgs({ mode: 'write' });
  const tools = a[a.indexOf('--tools') + 1];
  assert.equal(tools, 'Skill,Read,Glob,Grep,Edit,Write');
  assert.ok(a.includes('Bash'), 'Bash must appear only in the disallowed list');
  assert.ok(a.indexOf('Bash') > a.indexOf('--disallowed-tools'));
  assert.match(a.join(' '), /--permission-mode acceptEdits/);
});

test('claude never receives the flags that would disable skills or break auth', () => {
  const a = claudeArgs({ mode: 'read' }).join(' ');
  for (const forbidden of ['--safe-mode', '--bare', '--disable-slash-commands', '--add-dir']) {
    assert.ok(!a.includes(forbidden), `${forbidden} must never be passed`);
  }
  assert.match(a, /--setting-sources project/, 'project skills must load');
});

test('codex passes exactly two -c overrides', () => {
  const a = codexArgs({ mode: 'read', cwd: '/w', schemaPath: '/s.json', outPath: '/o.json', model: 'm', effort: 'high' });
  const cs = a.filter((x, i) => a[i - 1] === '-c');
  assert.deepEqual(cs, ['model="m"', 'model_reasoning_effort="high"']);
  assert.equal(a.filter((x) => x === '-c').length, 2);
});

test('codex sets reasoning effort even without a model, because --ignore-user-config resets it', () => {
  const a = codexArgs({ mode: 'read', cwd: '/w', outPath: '/o.json' });
  assert.ok(a.join(' ').includes('model_reasoning_effort="high"'));
  assert.ok(a.includes('--ignore-user-config') && a.includes('--ignore-rules'));
});

test('codex sandbox follows the mode and never skips the git check', () => {
  assert.ok(codexArgs({ mode: 'read', cwd: '/w', outPath: '/o' }).join(' ').includes('--sandbox read-only'));
  assert.ok(codexArgs({ mode: 'write', cwd: '/w', outPath: '/o' }).join(' ').includes('--sandbox workspace-write'));
  assert.ok(!codexArgs({ mode: 'write', cwd: '/w', outPath: '/o' }).includes('--skip-git-repo-check'));
});

// ---------------------------------------------------------------- parsing

test('lastJsonObject survives a leading banner and braces inside strings', () => {
  const o = lastJsonObject('Loading config...\n{"a":"} not the end {","b":2}\n');
  assert.deepEqual(o, { a: '} not the end {', b: 2 });
});

test('stripFences removes a json code fence', () => {
  assert.equal(stripFences('```json\n{"a":1}\n```'), '{"a":1}');
  assert.equal(stripFences('  {"a":1}  '), '{"a":1}');
});

// ---------------------------------------------------------------- taxonomy

const result = (o) => JSON.stringify({ type: 'result', ...o });

test('claude success is is_error === false, never subtype', () => {
  // The binary emits subtype:"success" alongside is_error:true; branching on
  // subtype would report a hard auth failure as a successful turn.
  const c = classifyClaude({
    stdout: result({ subtype: 'success', is_error: true, result: 'Failed to authenticate: OAuth session expired and could not be refreshed' }),
    stderr: '', killed: false, schemaRequested: true,
  });
  assert.equal(c.failure, 'AUTH');
});

test('claude quota is distinguished from a transient rate limit', () => {
  const hard = classifyClaude({ stdout: result({ is_error: true, result: 'usage limit reached' }), stderr: '', killed: false, schemaRequested: false });
  assert.equal(hard.failure, 'QUOTA'); assert.equal(hard.hard, true);
  const soft = classifyClaude({ stdout: result({ is_error: true, result: 'overloaded', api_error_status: 529 }), stderr: '', killed: false, schemaRequested: false });
  assert.equal(soft.failure, 'QUOTA'); assert.equal(soft.hard, false);
});

test('claude schema exhaustion is SCHEMA, not PROCESS', () => {
  const c = classifyClaude({
    stdout: result({ subtype: 'error_max_structured_output_retries', terminal_reason: 'structured_output_retry_exhausted' }),
    stderr: '', killed: false, schemaRequested: true,
  });
  assert.equal(c.failure, 'SCHEMA');
});

test('claude success without structured output is SCHEMA', () => {
  const c = classifyClaude({ stdout: result({ is_error: false, result: 'here you go' }), stderr: '', killed: false, schemaRequested: true });
  assert.equal(c.failure, 'SCHEMA');
  const ok = classifyClaude({ stdout: result({ is_error: false, result: 'x', structured_output: { a: 1 } }), stderr: '', killed: false, schemaRequested: true });
  assert.equal(ok.failure, 'OK');
  assert.deepEqual(ok.parsed.data, { a: 1 });
});

test('a benign codex JSONL error event on an exit-0 run is NOT a failure', () => {
  // Observed live: a fully successful run in a repo with a large skill catalog
  // emits this. Treating it as failure would fail every run in the target repos.
  const stdout = [
    JSON.stringify({ type: 'thread.started', thread_id: 't1' }),
    JSON.stringify({ type: 'item.completed', item: { id: 'item_0', type: 'error', message: 'Skill descriptions were shortened to fit the skills context budget' } }),
    JSON.stringify({ type: 'turn.completed', usage: { input_tokens: 10, output_tokens: 2 } }),
  ].join('\n');
  const out = `${process.cwd()}/.rl-test-out.json`;
  fs.writeFileSync(out, '{"ok":true}');
  const c = classifyCodex({ stdout, stderr: '', exitCode: 0, killed: false, outPath: out, schemaRequested: true });
  assert.equal(c.failure, 'OK');
  assert.deepEqual(c.parsed.data, { ok: true });
  assert.equal(c.parsed.sessionId, 't1');
  fs.rmSync(out, { force: true });
});

test('a codex schema-file rejection is fatal and must not be repaired or failed over', () => {
  const c = classifyCodex({ stdout: '', stderr: 'error: failed to parse schema file', exitCode: 1, killed: false, outPath: '/nope', schemaRequested: true });
  assert.equal(c.failure, 'PROCESS');
  assert.equal(c.fatal, true);
});

test('codex quota and auth are recognised from stderr', () => {
  assert.equal(classifyCodex({ stdout: '', stderr: 'You have hit your usage limit', exitCode: 1, killed: false, outPath: '/x' }).failure, 'QUOTA');
  assert.equal(classifyCodex({ stdout: '', stderr: 'not logged in; run codex login', exitCode: 1, killed: false, outPath: '/x' }).failure, 'AUTH');
});

test('a timeout is classified without parsing anything', () => {
  assert.equal(classifyClaude({ stdout: 'garbage', stderr: '', killed: true }).failure, 'TIMEOUT');
  assert.equal(classifyCodex({ stdout: '', stderr: '', exitCode: null, killed: true, outPath: '/x' }).failure, 'TIMEOUT');
});

// ---------------------------------------------------------------- ring

const ring = () => ({ providers: newProviderRing(['claude', 'codex']), activeProvider: 'codex' });

test('a hard quota puts a provider in cooldown and hands over to the other', () => {
  const st = ring();
  noteFailure(st, 'codex', { failure: 'QUOTA', hard: true, detail: 'usage limit reached' }, 20);
  assert.equal(st.providers.codex.status, 'COOLDOWN');
  assert.deepEqual(readyProviders(st), ['claude']);
  assert.equal(pickProvider(st), 'claude');
});

test('a soft quota does not remove the provider from the ring', () => {
  const st = ring();
  noteFailure(st, 'codex', { failure: 'QUOTA', hard: false }, 20);
  assert.deepEqual(readyProviders(st).sort(), ['claude', 'codex']);
});

test('an auth failure kills the provider permanently for this run', () => {
  const st = ring();
  noteFailure(st, 'claude', { failure: 'AUTH', detail: 'OAuth session expired' });
  assert.equal(st.providers.claude.status, 'DEAD');
  noteFailure(st, 'codex', { failure: 'AUTH' });
  assert.equal(pickProvider(st), null, 'both dead means the run has no provider');
});

test('an expired cooldown returns the provider to the ring', () => {
  const st = ring();
  st.providers.codex.status = 'COOLDOWN';
  st.providers.codex.cooldownUntil = new Date(Date.now() - 1000).toISOString();
  assert.ok(readyProviders(st).includes('codex'));
  assert.equal(st.providers.codex.status, 'READY');
});

test('review prefers the provider that did not write, and degrades gracefully', () => {
  const st = ring();
  assert.equal(pickReviewer(st, 'claude'), 'codex');
  st.providers.codex.status = 'DEAD';
  assert.equal(pickReviewer(st, 'claude'), 'claude', 'same-provider fresh session is the fallback');
});

// ---------------------------------------------------------------- privilege

test('only the two write phases may write; everything else fails closed', () => {
  assert.deepEqual([...WRITE_PHASES].sort(), ['characterization', 'execute']);
  for (const p of ['audit', 'deep_check', 'preflight', 'review', 'handoff', 'doctor', 'typo', '']) {
    assert.equal(modeFor(p), 'read', `${p} must not be writable`);
  }
});

test('a request that claims write for a non-write phase is refused, not granted', async () => {
  // Regression: an earlier version inferred the mode from a set of READ phase
  // names, so an unrecognised phase (a typo, or a new one like "doctor") got
  // Edit/Write and a workspace-write sandbox by default. Privilege must fail
  // closed, and asking for it wrongly must be an error rather than a grant.
  await assert.rejects(
    () => callProvider('codex', { phase: 'doctor', mode: 'write', cwd: '/tmp', body: '', runDir: '/tmp', attempt: 'primary' }, { agents: {} }, null),
    /not in WRITE_PHASES/,
  );
});

test('an unstated mode defaults to read rather than write', () => {
  const a = claudeArgs({ mode: 'read' });
  assert.equal(a[a.indexOf('--tools') + 1], 'Skill,Read,Glob,Grep');
});

test('effort is chosen per phase, and config can override it', async () => {
  const { effortFor, PHASE_EFFORT } = await import('../src/provider.mjs');
  // The phases where a weaker answer costs correctness keep the full budget.
  for (const p of ['audit', 'deep_check', 'execute', 'review']) assert.equal(effortFor(p, {}), 'high');
  // The ones that are second opinions or mechanical do not.
  assert.equal(effortFor('preflight', {}), 'medium');
  assert.equal(effortFor('handoff', {}), 'low');
  // A blanket agent-level setting still wins, and a per-phase one wins over that.
  assert.equal(effortFor('audit', { effort: 'low' }), 'low');
  assert.equal(effortFor('audit', { effort: 'low', effort_by_phase: { audit: 'max' } }), 'max');
  // An unknown phase must not silently become expensive.
  assert.equal(effortFor('nonesuch', {}), 'medium');
  assert.ok(Object.keys(PHASE_EFFORT).length >= 7);
});

test('every claude phase can actually invoke a skill', () => {
  // Regression: --tools was restricted to Read,Glob,Grep, which omits Skill and
  // therefore disables /skill-name entirely. Verified live: a session without it
  // reports NONE of the installed skills, while every phase still "succeeds" —
  // the whole sharpen-* routing layer silently does nothing.
  for (const mode of ['read', 'write']) {
    const a = claudeArgs({ mode });
    const tools = a[a.indexOf('--tools') + 1].split(',');
    assert.ok(tools.includes('Skill'), `${mode} mode must be able to invoke skills`);
    const denied = a.slice(a.indexOf('--disallowed-tools') + 1, a.indexOf('--setting-sources'));
    assert.ok(!denied.includes('Skill'), `${mode} mode must not deny Skill`);
  }
});

test('the operator\'s provider order is honoured, not the array order', () => {
  // Regression: readyProviders iterated the PROVIDERS constant, whose order is
  // alphabetical accident. Observed live — `--provider codex` produced
  // `claude(4) → codex(2)`, i.e. the flag did nothing on the first call.
  const st = { providers: newProviderRing(['claude', 'codex']), activeProvider: null, providerOrder: ['codex', 'claude'] };
  assert.deepEqual(readyProviders(st), ['codex', 'claude']);
  assert.equal(pickProvider(st), 'codex', 'the requested primary must lead');

  const other = { providers: newProviderRing(['claude', 'codex']), activeProvider: null, providerOrder: ['claude', 'codex'] };
  assert.equal(pickProvider(other), 'claude');

  // Absent an order (older state files), fall back to the constant rather than crash.
  const legacy = { providers: newProviderRing(['claude', 'codex']), activeProvider: null };
  assert.equal(pickProvider(legacy), 'claude');
});

test('a single-provider order never returns the excluded one', () => {
  const st = { providers: newProviderRing(['claude', 'codex']), activeProvider: null, providerOrder: ['codex'] };
  assert.deepEqual(readyProviders(st), ['codex']);
  assert.equal(pickReviewer(st, 'codex'), 'codex', 'degrades to a same-provider fresh session');
});
