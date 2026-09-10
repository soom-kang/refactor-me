import { test } from 'node:test';
import assert from 'node:assert/strict';
import { renderSummary, renderMarkdown } from '../src/report.mjs';

const base = {
  runId: 'r', status: 'DONE_PARTIAL', reason: 'x', durationMinutes: 16,
  repoRoot: '/r', baseCommit: 'd855583aabb', baseBranch: 'main',
  branch: 'refactor/auto-r', worktree: '/w',
  counters: { cycles: 1, commits: 1, violations: 0 },
  commits: [{ oid: 'a1b2c3d', category: 'DEAD_CODE', subject: 's', paths: [] }],
  skipped: [], validation: { describe: 'GREEN 3 / RED 1 of 4', ran: [], notRun: [] },
};

test('the provider arrow follows the order the run actually used', () => {
  // Regression: the arrow was rendered from the providers object's key order, so
  // a run that went codex → claude printed "claude(2) → codex(5 COOLDOWN)" —
  // exactly backwards, on a symbol that means chronology.
  const out = renderSummary({
    ...base,
    providerOrder: ['codex', 'claude'],
    providers: {
      claude: { status: 'READY', calls: 2, quotaHits: 0, lastError: null },
      codex: { status: 'COOLDOWN', calls: 5, quotaHits: 1, lastError: 'quota' },
    },
  }, '/run');
  assert.match(out, /provider\s+codex\(5 COOLDOWN\) → claude\(2\)/);
});

test('a provider that was never called is omitted', () => {
  const out = renderSummary({
    ...base,
    providerOrder: ['codex', 'claude'],
    providers: {
      claude: { status: 'DISABLED', calls: 0, quotaHits: 0, lastError: null },
      codex: { status: 'READY', calls: 7, quotaHits: 0, lastError: null },
    },
  }, '/run');
  assert.match(out, /provider\s+codex\(7\)$/m);
  assert.ok(!out.includes('claude'));
});

test('an older state without providerOrder still renders', () => {
  const out = renderSummary({
    ...base,
    providers: { codex: { status: 'READY', calls: 3, quotaHits: 0, lastError: null } },
  }, '/run');
  assert.match(out, /provider\s+codex\(3\)/);
});

// --------------------------------------------------------------- usage

// renderMarkdown needs the full report shape; renderSummary tolerates less.
const mdBase = {
  ...base,
  providers: { codex: { status: 'READY', calls: 4, quotaHits: 0, lastError: null } },
  providerOrder: ['codex'],
};

const usageOf = (o) => ({
  processes: 0, calls: 0, ms: 0, inputTokens: 0, outputTokens: 0,
  costUsd: null, costMissing: 0, failedCalls: 0, ...o,
});

test('a codex-only run is never priced at zero dollars', () => {
  // The failure this guards is quiet and total: $0.00 reads as "it was free"
  // rather than "nobody said". Every team member reading a report would
  // believe it.
  const totals = usageOf({ processes: 4, calls: 4, ms: 1_320_000, inputTokens: 711_300, outputTokens: 37_220, costMissing: 4 });
  const j = { ...mdBase, providerMinutes: 22, usage: { totals, byPhase: { audit: usageOf({ ...totals }) } } };
  const md = renderMarkdown(j);
  assert.match(md, /## Cost and time/);
  assert.match(md, /Cost: \*\*Not reported\*\*/);
  assert.ok(!md.includes('$0.00'), md);
  assert.match(renderSummary(j, '/run'), /cost not reported/);
});

test('a mixed run reports a floor, not a total', () => {
  const totals = usageOf({ processes: 2, calls: 2, ms: 940_000, inputTokens: 500_000, outputTokens: 29_000, costUsd: 1.41, costMissing: 1 });
  const j = { ...mdBase, providerMinutes: 16, usage: { totals, byPhase: { execute: usageOf({ ...totals }) } } };
  assert.match(renderMarkdown(j), /Cost: \*\*\$1\.41 or more\*\*/);
  assert.match(renderSummary(j, '/run'), /\$1\.41\+/);
});

test('a fully priced run just says the number', () => {
  const totals = usageOf({ processes: 1, calls: 1, ms: 372_000, inputTokens: 210_400, outputTokens: 8_100, costUsd: 0.51 });
  const j = { ...mdBase, providerMinutes: 6, usage: { totals, byPhase: { audit: usageOf({ ...totals }) } } };
  const md = renderMarkdown(j);
  assert.match(md, /Cost: \*\*\$0\.51\*\*/);
  assert.ok(!md.includes('or more'), md);
});

test('a run that called no provider gets no cost section at all', () => {
  // An ABORTED run should not be handed an empty table to interpret.
  const j = { ...mdBase, providerMinutes: 0, usage: { totals: usageOf({}), byPhase: {} } };
  assert.ok(!renderMarkdown(j).includes('## Cost and time'));
  assert.ok(!renderSummary(j, '/run').includes('usage'));
});

test('a report written before usage existed still renders', () => {
  // Same guarantee as the providerOrder case above: old runs in .refactor/runs
  // must stay readable, and nothing may leak the word undefined.
  const out = renderSummary({ ...base, providerOrder: ['codex'], providers: { codex: { status: 'READY', calls: 3, quotaHits: 0 } } }, '/run');
  assert.ok(!out.includes('undefined'), out);
  assert.ok(!out.includes('usage'), out);
  assert.ok(!renderMarkdown(mdBase).includes('undefined'));
});

test('the summary leads with the build that produced the run', () => {
  // This block is what a beta tester screenshots; without the version the
  // feedback cannot be attributed to a build.
  assert.match(renderSummary({ ...mdBase, toolVersion: '0.1.0' }, '/run'), /^refactor-me 0\.1\.0 · Partially completed/);
  assert.match(renderMarkdown({ ...mdBase, toolVersion: '0.1.0' }), /- Tool version: refactor-me 0\.1\.0/);
});

test('a run from before versioning still renders, without leaking undefined', () => {
  const out = renderSummary(mdBase, '/run');
  assert.match(out, /^refactor-me Partially completed/);
  assert.ok(!out.includes('undefined'), out);
  assert.match(renderMarkdown(mdBase), /- Tool version: refactor-me Not recorded/);
});

for (const [status, en, ko] of [
  ['DONE', 'Completed', '완료'], ['NO_CHANGES', 'No changes', '변경 없음'],
  ['DONE_PARTIAL', 'Partially completed', '부분 완료'], ['ABORTED', 'Aborted', '중단'],
  ['HALTED_UNSAFE', 'Safety halt', '안전 정지'],
]) {
  test(`both languages preserve evidence for ${status}`, () => {
    const report = { ...mdBase, status, reason: 'no eligible candidates remain',
      skipped: [{ reason: 'RISK_UNKNOWN', detail: 'Original evidence 31415' }, { reason: 'NEW_CODE', detail: 'raw error' }] };
    const before = JSON.stringify(report);
    for (const [lang, title] of [['en', en], ['ko', ko]]) {
      const md = renderMarkdown(report, lang);
      assert.ok(md.includes(title));
      assert.ok(renderSummary(report, '/run', lang).includes(title));
      for (const text of ['Original evidence 31415', 'NEW_CODE', 'raw error', 'a1b2c3d', 'GREEN 3 / RED 1 of 4']) assert.ok(md.includes(text), text);
      assert.equal(md.includes('증거로 보존한 worktree'), lang === 'ko' && status === 'HALTED_UNSAFE');
    }
    assert.equal(JSON.stringify(report), before);
    assert.match(renderMarkdown(report, 'en'), /Insufficient evidence to assess risk/);
    assert.match(renderMarkdown(report, 'ko'), /위험도 판정 근거 부족/);
  });
}

test('Korean reports retain unknown and partial cost semantics', () => {
  const totals = usageOf({ processes: 2, calls: 2, ms: 1000, inputTokens: 456, outputTokens: 123, costMissing: 2 });
  const report = { ...mdBase, usage: { totals, byPhase: {} } };
  assert.match(renderMarkdown(report, 'ko'), /비용: \*\*미보고/);
  report.usage.totals = { ...totals, costUsd: 1.41, costMissing: 1 };
  assert.match(renderMarkdown(report, 'ko'), /비용: \*\*\$1\.41 이상/);
  assert.match(renderMarkdown(report), /Cost: \*\*\$1\.41 or more/);
  for (const lang of ['en', 'ko']) {
    const md = renderMarkdown(report, lang);
    assert.ok(md.includes('456') && md.includes('123'));
    assert.ok(!md.includes('$0.00'));
  }
});

test('buildReport writes one selected Markdown language and language-neutral JSON', async (t) => {
  const fs = await import('node:fs');
  const os = await import('node:os');
  const path = await import('node:path');
  const { buildReport } = await import('../src/report.mjs');
  const runDir = fs.mkdtempSync(path.join(os.tmpdir(), 'refactor-me-report-'));
  t.after(() => fs.rmSync(runDir, { recursive: true, force: true }));
  const context = {
    runDir, startedAt: Date.now(), commands: [], skippedCommands: [], hints: [], baseline: { describe: 'GREEN 1' },
    state: {
      runId: 'r', terminal: { status: 'NO_CHANGES', reason: 'no eligible candidates remain' },
      repoRoot: '/repo', baseOid: 'abcdef0', baseBranch: 'main', worktree: '/worktree', providers: {},
      counters: { commits: 0, cycles: 0 }, commits: [], seen: { skipped: [] },
    },
  };
  const english = buildReport(context);
  assert.match(fs.readFileSync(path.join(runDir, 'report.md'), 'utf8'), /## Committed changes/);
  const korean = buildReport(context, 'ko');
  assert.match(fs.readFileSync(path.join(runDir, 'report.md'), 'utf8'), /## 커밋된 변경/);
  assert.deepEqual(english, korean);
  assert.deepEqual(fs.readdirSync(runDir).sort(), ['report.json', 'report.md']);
  assert.throws(() => buildReport(context, 'fr'), /unsupported language/);
});

test('Korean terminal reasons translate known limits while preserving raw details', () => {
  for (const [reason, expected] of [
    ['stopped on commit budget (1)', '커밋 한도 1개'],
    ['stopped on cycle budget (25)', '사이클 한도 25회'],
    ['stopped on wall clock (180m)', '경과 시간 한도 180분'],
    ['stopped on 3 consecutive failures', '후보가 3회 연속 실패'],
    ['stopped on all providers exhausted', '사용 가능한 프로바이더가 없습니다'],
    ['REGRESSION: src/a.ts:42 E123', '검증 회귀 (REGRESSION): src/a.ts:42 E123'],
    ['unknown error 그대로', 'unknown error 그대로'],
  ]) {
    const report = { ...mdBase, reason };
    assert.ok(renderMarkdown(report, 'ko').includes(expected));
    assert.ok(renderMarkdown(report).includes(reason));
  }
});
