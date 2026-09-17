import { test } from 'node:test';
import assert from 'node:assert/strict';
import { rank, DEFAULT_POLICY, assertPolicy, packetRiskVerdict, violatedPaths, noCandidatesReason } from '../src/loop.mjs';
import { newState, newProviderRing, markSkipped, markDone, noteAttempt, isSeen } from '../src/state.mjs';

const cand = (o) => ({
  candidate_id: 'c', category: 'DEAD_CODE', risk_level: 'L0_LOW', readiness: 'READY',
  related_files: ['src/a.mjs'], estimated_file_count: 1, primary_symbol: 'A', problem: '', ...o,
});

function ctx(overrides = {}, targets = []) {
  const state = newState({
    id: 'r', repoRoot: '/r', worktree: '/w', baseOid: 'a', baseBranch: 'main',
    branchName: 'b', providers: newProviderRing(['codex']), targets,
  });
  const logged = [];
  return {
    state, policy: { ...DEFAULT_POLICY, ...overrides }, targets,
    log: { info: (m) => logged.push(m) }, store: { save() {} }, logged,
  };
}

test('L3 is never executed, whatever the model says', () => {
  const c = ctx();
  const out = rank(c, [cand({ candidate_id: 'x', risk_level: 'L3_CRITICAL' })]);
  assert.equal(out.length, 0);
  assert.equal(c.state.seen.skipped[0].reason, 'RISK_EXCLUDED');
});

// sharpen-assess reports `change_risk: unknown` when the evidence cannot support a
// level. The failure this guards against is coercion: a schema without UNKNOWN
// pushes the model onto the nearest value, and the nearest cheap value is the
// one that gets executed.
test('an unknown risk level is set aside for a human, not executed', () => {
  const c = ctx();
  const out = rank(c, [cand({ candidate_id: 'x', risk_level: 'UNKNOWN' })]);
  assert.equal(out.length, 0);
  assert.equal(c.state.seen.skipped[0].reason, 'RISK_UNKNOWN',
    'UNKNOWN is missing evidence, not a risk level outside the allowed range');
});

test('UNKNOWN cannot be enabled by widening allowed_risks', () => {
  const c = ctx({ allowed_risks: ['L0_LOW', 'L1_MODERATE', 'L2_HIGH', 'L3_CRITICAL', 'UNKNOWN'] });
  assert.equal(rank(c, [cand({ candidate_id: 'x', risk_level: 'UNKNOWN' })]).length, 0);
  assert.equal(c.state.seen.skipped[0].reason, 'RISK_UNKNOWN');
});

test('a candidate the model itself rejected is not retried', () => {
  const c = ctx();
  assert.equal(rank(c, [cand({ readiness: 'REJECT' })]).length, 0);
  assert.equal(c.state.seen.skipped[0].reason, 'MODEL_REJECTED');
});

test('an oversized candidate is filtered before it can be planned', () => {
  const c = ctx({ max_files_per_candidate: 3 });
  assert.equal(rank(c, [cand({ estimated_file_count: 9 })]).length, 0);
  assert.equal(c.state.seen.skipped[0].reason, 'TOO_LARGE');
});

test('completed and abandoned work is never re-proposed, even reworded', () => {
  const c = ctx();
  const first = cand({ candidate_id: 'orig', title: 'remove the old parser' });
  const eligible = rank(c, [first]);
  assert.equal(eligible.length, 1);
  markDone(c.state, eligible[0].fp);

  // Same kind, same paths, same symbol — different prose. Must still collide.
  const reworded = cand({ candidate_id: 'other', title: 'delete legacy parsing helper' });
  assert.equal(rank(c, [reworded]).length, 0);
});

test('a candidate stopped by provider exhaustion stays eligible', () => {
  // The whole cross-provider handoff depends on this: running out of quota tells
  // us nothing about the candidate, so it must not be recorded as abandoned.
  const c = ctx();
  const [picked] = rank(c, [cand()]);
  noteAttempt(c.state, picked.fp);                       // one attempt spent
  assert.ok(!isSeen(c.state, picked.fp), 'an attempt alone must not mark it seen');
  assert.equal(rank(c, [cand()]).length, 1, 'still offered to the other provider');
});

test('a candidate is abandoned once its attempts are spent', () => {
  const c = ctx({ max_attempts_per_fingerprint: 2 });
  const [picked] = rank(c, [cand()]);
  noteAttempt(c.state, picked.fp);
  noteAttempt(c.state, picked.fp);
  assert.equal(rank(c, [cand()]).length, 0);
  assert.equal(c.state.seen.skipped.at(-1).reason, 'ATTEMPTS_EXHAUSTED');
});

test('ranking prefers lower risk, then readiness, then a smaller blast radius', () => {
  const c = ctx();
  const out = rank(c, [
    cand({ candidate_id: 'big', risk_level: 'L0_LOW', estimated_file_count: 5, related_files: ['src/e.mjs'], primary_symbol: 'E' }),
    cand({ candidate_id: 'risky', risk_level: 'L2_HIGH', related_files: ['src/b.mjs'], primary_symbol: 'B' }),
    cand({ candidate_id: 'small', risk_level: 'L0_LOW', estimated_file_count: 1, related_files: ['src/c.mjs'], primary_symbol: 'C' }),
    cand({ candidate_id: 'unsure', risk_level: 'L0_LOW', readiness: 'NEEDS_EVIDENCE', estimated_file_count: 1, related_files: ['src/d.mjs'], primary_symbol: 'D' }),
  ]);
  assert.deepEqual(out.map((x) => x.candidate_id), ['small', 'big', 'unsure', 'risky']);
});

test('a skipped candidate records the paths a later audit is warned about', () => {
  const c = ctx();
  const [picked] = rank(c, [cand({ related_files: ['src/a.mjs', 'src/b.mjs'] })]);
  markSkipped(c.state, picked.fp, 'OUT_OF_SCOPE', { detail: 'touched a lockfile', paths: ['pnpm-lock.yaml'] });
  assert.deepEqual(c.state.seen.skipped[0].detail.paths, ['pnpm-lock.yaml']);
});

test('commit subjects do not double the verb', async () => {
  const { commitSubject } = await import('../src/loop.mjs');
  // Observed live: "refactor(src): remove Remove the unreachable legacy parser".
  // Titles arrive already phrased as an action, so a category verb must only be
  // supplied when the title does not open with one.
  assert.equal(
    commitSubject({ title: 'Remove the unreachable legacy parser module', category: 'DEAD_CODE', allowlist: ['src/legacy-parser.mjs'] }),
    'refactor(src): remove the unreachable legacy parser module');
  assert.equal(
    commitSubject({ title: 'the duplicated status label lookup', category: 'DEDUPLICATION', allowlist: ['src/status-a.mjs'] }),
    'refactor(src): consolidate the duplicated status label lookup');
  assert.equal(
    commitSubject({ title: 'Split big-panel along its value seam.', category: 'LARGE_COMPONENT_SPLIT', allowlist: ['app/web/big.tsx'] }),
    'refactor(app): split big-panel along its value seam');
  // Falls back to the candidate id rather than producing an empty subject.
  assert.match(commitSubject({ candidate_id: 'C-007', category: 'DEAD_CODE', allowlist: [] }), /refactor\(core\): remove C-007/);
});

test('a non-Latin title is used verbatim rather than given an English verb', async () => {
  const { commitSubject } = await import('../src/loop.mjs');
  // Observed live: "refactor(src): remove 참조되지 않는 ... 삭제" — the Korean
  // title already ends in its verb, and the English regex could not see it, so
  // the subject read "remove ... delete". We cannot parse arbitrary grammar, so
  // anything outside Latin script is left alone.
  assert.equal(
    commitSubject({ title: '참조되지 않는 status-b 및 status-c 모듈 삭제', category: 'DEAD_CODE', allowlist: ['src/status-b.mjs'] }),
    'refactor(src): 참조되지 않는 status-b 및 status-c 모듈 삭제');
  // A trailing ideographic full stop is trimmed like a period.
  assert.equal(
    commitSubject({ title: '중복 로직 통합。', category: 'DEDUPLICATION', allowlist: ['src/a.mjs'] }),
    'refactor(src): 중복 로직 통합');
});

test('preflight log lines lead with the verdict, not the objection', async () => {
  // Regression: a passing preflight logged "hypothesis falsified: <the danger>",
  // which reads as "here is a problem". A handoff brief generated from that
  // journal reported the candidate as BLOCKED when preflight had cleared it.
  const src = await import('node:fs').then((fs) =>
    fs.readFileSync(new URL('../src/loop.mjs', import.meta.url), 'utf8'));
  assert.match(src, /log\.pass\(`CLEARED — strongest objection ruled out/);
  assert.match(src, /log\.info\(`BLOCKED — /);
  assert.ok(!src.includes('hypothesis falsified:'), 'the ambiguous wording must be gone');
});

test('the handoff brief declares itself a mid-run snapshot in either language', async () => {
  const { renderHandoffHeader } = await import('../src/report.mjs');
  for (const language of ['en', 'ko']) {
    const header = renderHandoffHeader('codex', 'claude', 'QUOTA', language);
    assert.match(header, /not the final outcome/);
    assert.match(header, /report\.md/);
    assert.match(header, /codex → claude/);
    assert.match(header, language === 'ko' ? /전환 시점의 스냅샷입니다/ : /Snapshot at/);
  }
});

// ---------------------------------------------------------------- --target

// The audit prompt asks for in-scope candidates, but a prompt is a request, not
// a guarantee; this is where the request is enforced.
test('a candidate with no file inside the target is set aside', () => {
  const c = ctx({}, ['app/web']);
  const out = rank(c, [cand({ candidate_id: 'x', related_files: ['app/api/main.go'] })]);
  assert.equal(out.length, 0);
  assert.equal(c.state.seen.skipped[0].reason, 'OUT_OF_TARGET');
});

// The point of the feature. A dead export in app/web is deleted together with
// its callers and tests elsewhere; requiring every path to be in scope would
// reject exactly the candidates worth executing and leave the tree broken.
test('one file inside the target is enough, however many lie outside', () => {
  const c = ctx({}, ['app/web']);
  const out = rank(c, [cand({
    candidate_id: 'x',
    related_files: ['app/web/lib/dead.ts', 'src/caller.ts', 'src/caller.test.ts'],
    estimated_file_count: 3,
  })]);
  assert.equal(out.length, 1, 'callers outside the target are legitimate collateral');
});

// A scope filter that mis-answers the unscoped case starves every ordinary run
// without changing a single log line that would explain why.
test('an unscoped run filters nothing on scope grounds', () => {
  const c = ctx();
  const out = rank(c, [cand({ candidate_id: 'x', related_files: ['anywhere/at/all.ts'] })]);
  assert.equal(out.length, 1);
  assert.equal(c.state.seen.skipped.length, 0);
});

// app/web must not swallow app/website — the filter uses the same containment
// rule as the prompt inventory, and this pins them together.
test('the target filter does not match a sibling with a longer name', () => {
  const c = ctx({}, ['app/web']);
  const out = rank(c, [cand({ candidate_id: 'x', related_files: ['app/website/a.ts'] })]);
  assert.equal(out.length, 0);
  assert.equal(c.state.seen.skipped[0].reason, 'OUT_OF_TARGET');
});

// ---------------------------------------------------------------- unknown_risk

test('the shipped default sets an unknown risk aside', () => {
  assert.equal(DEFAULT_POLICY.unknown_risk, 'set_aside',
    'a run that upgrades and changes nothing must behave exactly as it did yesterday');
});

// The real candidates this flag exists for: a split whose only open question is
// whether any other file depends on its position in a global test order. The
// model cannot state a risk level without that check, and DEEP_CHECK is the
// phase whose whole job is running it.
test('an UNKNOWN candidate that names its missing check reaches deep check when the policy says so', () => {
  const c = ctx({ unknown_risk: 'deep_check' });
  const out = rank(c, [cand({
    candidate_id: 'mobile-ui', risk_level: 'UNKNOWN', readiness: 'NEEDS_EVIDENCE',
    estimated_file_count: 5, problem: 'cross-file ordering in tests/e2e is unverified',
  })]);
  assert.equal(out.length, 1);
  assert.equal(c.state.seen.skipped.length, 0, 'nothing was set aside');
});

// The trap: UNKNOWN is not a member of allowed_risks, so a pass-through written
// as a fall-through lands on the very next check and is re-rejected one line
// later as RISK_EXCLUDED. Only the skipped-length assertion catches that.
test('a passed-through UNKNOWN does not fall into the allowed_risks test', () => {
  const c = ctx({ unknown_risk: 'deep_check' });
  rank(c, [cand({ risk_level: 'UNKNOWN', readiness: 'NEEDS_EVIDENCE' })]);
  assert.equal(c.state.seen.skipped.length, 0);
  assert.ok(!c.logged.some((m) => m.includes('RISK_EXCLUDED')), c.logged.join('\n'));
});

test('deep_check passes through exactly one shape', () => {
  for (const readiness of ['READY', 'REJECT']) {
    const c = ctx({ unknown_risk: 'deep_check' });
    assert.equal(rank(c, [cand({ risk_level: 'UNKNOWN', readiness })]).length, 0, readiness);
    assert.equal(c.state.seen.skipped[0].reason, 'RISK_UNKNOWN',
      'a model that claims readiness while unable to size the danger is inconsistent, not short of one check');
  }
});

test('a stated risk outside the policy is still excluded under deep_check', () => {
  const c = ctx({ unknown_risk: 'deep_check' });
  assert.equal(rank(c, [cand({ risk_level: 'L3_CRITICAL', readiness: 'NEEDS_EVIDENCE' })]).length, 0);
  assert.equal(c.state.seen.skipped[0].reason, 'RISK_EXCLUDED');
});

test('a passed-through UNKNOWN never outranks a stated risk level', () => {
  const c = ctx({ unknown_risk: 'deep_check' });
  const out = rank(c, [
    cand({ candidate_id: 'unsure', risk_level: 'UNKNOWN', readiness: 'NEEDS_EVIDENCE', primary_symbol: 'U' }),
    cand({ candidate_id: 'risky', risk_level: 'L2_HIGH', readiness: 'NEEDS_EVIDENCE', primary_symbol: 'R' }),
  ]);
  assert.deepEqual(out.map((x) => x.candidate_id), ['risky', 'unsure'],
    'a danger known to be high is better understood than one nobody could size');
});

test('an invalid unknown_risk stops the run before it starts', () => {
  assert.throws(() => assertPolicy({ ...DEFAULT_POLICY, unknown_risk: 'deep-check' }), /unknown_risk/);
  for (const mode of ['set_aside', 'deep_check']) {
    assert.doesNotThrow(() => assertPolicy({ ...DEFAULT_POLICY, unknown_risk: mode }));
  }
});

// ---------------------------------------------------------------- the packet's own risk

test('the packet risk gate reads the packet, not the audit', () => {
  assert.equal(packetRiskVerdict({ risk_level: 'L1_MODERATE' }, DEFAULT_POLICY), null);
  assert.equal(packetRiskVerdict({ risk_level: 'L3_CRITICAL' }, DEFAULT_POLICY).reason, 'RISK_EXCLUDED');
  assert.equal(packetRiskVerdict({ risk_level: 'UNKNOWN' }, DEFAULT_POLICY).reason, 'RISK_UNKNOWN');
});

// The assertion that keeps the flag from becoming an escape hatch: it buys the
// candidate a deep check, not a pass on the answer.
test('unknown_risk never relaxes the gate that runs after deep check', () => {
  const v = packetRiskVerdict({ risk_level: 'UNKNOWN' }, { ...DEFAULT_POLICY, unknown_risk: 'deep_check' });
  assert.equal(v.reason, 'RISK_UNKNOWN');
});

// ---------------------------------------------------------------- forbidden ground

test('a candidate filtered during ranking does not forbid its paths', () => {
  const c = ctx({ max_files_per_candidate: 3 });
  rank(c, [cand({ estimated_file_count: 9, related_files: ['src/a.mjs'] })]);
  assert.equal(c.state.seen.skipped[0].reason, 'TOO_LARGE');
  assert.deepEqual(violatedPaths(c.state), [],
    'nothing was attempted, so no ground is off limits to the next audit');
});

test('a path an attempt actually violated is forbidden', () => {
  const c = ctx();
  markSkipped(c.state, 'fp1', 'FORBIDDEN_PATH', { detail: 'x', paths: ['pnpm-lock.yaml'], violated: true });
  assert.deepEqual(violatedPaths(c.state), ['pnpm-lock.yaml']);
});

test('a skip record written before the flag existed forbids nothing', () => {
  const c = ctx();
  markSkipped(c.state, 'fp1', 'OUT_OF_SCOPE', { detail: 'x', paths: ['src/a.mjs'] });
  assert.deepEqual(violatedPaths(c.state), [],
    'under-warning is the safe direction for a list that removes ground from the audit');
});

// ---------------------------------------------------------------- diagnosis

test('the RISK_UNKNOWN record carries the audit own objection, not a skill name', () => {
  const c = ctx();
  rank(c, [cand({ risk_level: 'UNKNOWN', problem: 'cross-file ordering in tests/e2e is unverified' })]);
  const { detail } = c.state.seen.skipped[0];
  assert.match(detail.detail, /cross-file ordering/);
  assert.doesNotMatch(detail.detail, /sharpen-assess/,
    'the controller cannot know which skill ran, and the candidate already named the gap');

  const c2 = ctx();
  rank(c2, [cand({ risk_level: 'UNKNOWN', problem: '' })]);
  assert.doesNotMatch(c2.state.seen.skipped[0].detail.detail, /undefined/);
});

test('a cycle where everything was filtered says so', () => {
  const c = ctx();
  rank(c, [
    cand({ candidate_id: 'a', readiness: 'REJECT', primary_symbol: 'A' }),
    cand({ candidate_id: 'b', risk_level: 'UNKNOWN', primary_symbol: 'B' }),
  ]);
  const line = c.logged.find((m) => m.startsWith('all 2 candidate(s) filtered'));
  assert.ok(line, `no summary line was logged:\n${c.logged.join('\n')}`);
  assert.match(line, /MODEL_REJECTED 1/);
  assert.match(line, /RISK_UNKNOWN 1/);
});

test('the stop reason names which kind of empty audit ended the run', () => {
  assert.equal(noCandidatesReason(['ALL_FILTERED', 'ALL_FILTERED']), 'every proposed candidate was filtered by policy');
  assert.equal(noCandidatesReason(['NO_PROPOSALS']), 'the audit proposed no candidates');
  assert.equal(noCandidatesReason(['NO_PROPOSALS', 'ALL_FILTERED']), 'no eligible candidates remain');
  assert.equal(noCandidatesReason(undefined), 'no eligible candidates remain',
    'the original wording stays the fallback, so every existing report still renders');
});
