import { test } from 'node:test';
import assert from 'node:assert/strict';
import {
  globToRegex, matchesAny, FORBIDDEN_GLOBS, isTestPath, WEAKENING_RE,
  netSizeRule, runChecks, changedLineBudget, GATE_CHECK_COUNT,
} from '../src/gate.mjs';

// ---------------------------------------------------------------- globs

test('glob: **/ matches at any depth including root', () => {
  const r = globToRegex('**/package.json');
  assert.ok(r.test('package.json'));
  assert.ok(r.test('app/web/package.json'));
  assert.ok(!r.test('src/mypackage.json'), 'must not match a suffix of the basename');
});

test('glob: bare pattern matches the basename at any depth', () => {
  assert.ok(globToRegex('*.png').test('src/assets/logo.png'));
  assert.ok(globToRegex('LICENSE').test('vendor/LICENSE'));
});

test('glob: * does not cross a path separator', () => {
  assert.ok(!globToRegex('src/*.mjs').test('src/deep/a.mjs'));
  assert.ok(globToRegex('src/*.mjs').test('src/a.mjs'));
});

test('forbidden list covers every lockfile and oracle we care about', () => {
  const res = FORBIDDEN_GLOBS.map(globToRegex);
  for (const p of [
    'package-lock.json', 'app/web/pnpm-lock.yaml', 'app/api/go.sum', 'Cargo.lock',
    'src/__snapshots__/x.snap', 'e2e/golden/a.txt', 'app/api/testdata/in.json',
    '.github/workflows/ci.yml', 'db/migrations/001.sql', '.env.production',
    '.claude/skills/x/SKILL.md', 'AGENTS.md', 'api/openapi.yaml', 'gen/user.pb.go',
    'assets/hero.png', '.git/config',
  ]) assert.ok(matchesAny(p, res), `expected forbidden: ${p}`);

  for (const p of ['src/index.mjs', 'app/web/features/billing/Panel.tsx', 'README.md']) {
    assert.ok(!matchesAny(p, res), `must not be forbidden: ${p}`);
  }
});

// ---------------------------------------------------------------- test integrity

test('test path detection spans js/go/python conventions', () => {
  for (const p of ['src/a.test.ts', 'src/a.spec.js', 'tests/x.mjs', 'pkg/thing_test.go', 'app/test_utils.py']) {
    assert.ok(isTestPath(p), p);
  }
  assert.ok(!isTestPath('src/latest.ts'));
});

test('weakening tokens are recognised across languages', () => {
  for (const l of ['it.skip("x")', 'describe.only(', 'xit(', 'xdescribe(', 't.Skip()', '@pytest.mark.skip', '#[ignore]']) {
    assert.ok(WEAKENING_RE.test(l), l);
  }
  assert.ok(!WEAKENING_RE.test('const skipped = list.filter(Boolean)'));
});

// ---------------------------------------------------------------- net size

test('net size: a removal must actually remove', () => {
  assert.ok(netSizeRule('DEAD_CODE', { insertions: 2, deletions: 120 }).ok);
  assert.ok(!netSizeRule('DEAD_CODE', { insertions: 200, deletions: 10 }).ok);
});

test('net size: consolidation may not grow the codebase', () => {
  assert.ok(netSizeRule('DEDUPLICATION', { insertions: 10, deletions: 40 }).ok);
  assert.ok(!netSizeRule('DEDUPLICATION', { insertions: 41, deletions: 40 }).ok);
});

test('net size: a split redistributes, and must reach its stated goal', () => {
  const base = { insertions: 640, deletions: 620, originalLines: 620 };
  assert.ok(netSizeRule('LARGE_COMPONENT_SPLIT', { ...base, primaryFileLines: 180 }).ok);
  const drift = netSizeRule('LARGE_COMPONENT_SPLIT', { ...base, primaryFileLines: 540 });
  assert.ok(!drift.ok);
  assert.match(drift.detail, /SCOPE_DRIFT/);
  assert.ok(!netSizeRule('LARGE_COMPONENT_SPLIT', { insertions: 900, deletions: 620, originalLines: 620, primaryFileLines: 100 }).ok);
});

// ---------------------------------------------------------------- ordering

const PRE = 'a'.repeat(40);
const BASE = 'b'.repeat(40);

function facts(over = {}) {
  return {
    worktreeHead: PRE, sourceHead: BASE,
    sourceFpNow: 'fp', sourceFpBefore: 'fp',
    preOid: PRE, baseOid: BASE,
    changed: ['src/a.mjs'], deleted: [],
    insertions: 1, deletions: 30, binary: [],
    perFile: new Map([['src/a.mjs', { ins: 1, del: 30 }]]),
    addedByFile: {}, primaryFileLines: 40,
    prospectiveTree: 'c'.repeat(40),
    ...over,
  };
}
const PACKET = { category: 'DEAD_CODE', allowlist: ['src/a.mjs'] };
const POLICY = { max_files_per_candidate: 8, max_changed_lines: 600 };

test('clean candidate passes every controller check', () => {
  const r = runChecks(facts(), PACKET, POLICY, { treeHashes: [] });
  assert.equal(r.verdict, 'PASS');
  assert.ok(r.checks.every((c) => c.ok));
});

test('a moved worktree HEAD halts rather than merely failing', () => {
  const r = runChecks(facts({ worktreeHead: 'z'.repeat(40) }), PACKET, POLICY, {});
  assert.equal(r.verdict, 'HALT');
  assert.equal(r.violation.code, 'WORKTREE_HEAD_MOVED');
});

test('a mutated source repository halts', () => {
  const r = runChecks(facts({ sourceFpNow: 'other' }), PACKET, POLICY, {});
  assert.equal(r.verdict, 'HALT');
  assert.equal(r.violation.code, 'SOURCE_MUTATED');
});

test('an empty change set is NO_OP, not a violation', () => {
  const r = runChecks(facts({ changed: [] }), PACKET, POLICY, {});
  assert.equal(r.verdict, 'NO_OP');
});

test('first failure wins: forbidden path outranks the allowlist', () => {
  // pnpm-lock.yaml is both forbidden AND outside the allowlist; the reported
  // code must be the more dangerous one.
  const r = runChecks(
    facts({ changed: ['src/a.mjs', 'pnpm-lock.yaml'] }),
    PACKET, POLICY, { treeHashes: [] },
  );
  assert.equal(r.verdict, 'VIOLATION');
  assert.equal(r.violation.code, 'FORBIDDEN_PATH');
});

test('binary content is rejected regardless of extension', () => {
  const r = runChecks(facts({ binary: ['src/blob.dat'], changed: ['src/a.mjs', 'src/blob.dat'] }), PACKET, POLICY, { treeHashes: [] });
  assert.equal(r.violation.code, 'FORBIDDEN_BINARY');
});

test('a path outside the allowlist is a violation', () => {
  const r = runChecks(facts({ changed: ['src/a.mjs', 'src/b.mjs'] }), PACKET, POLICY, { treeHashes: [] });
  assert.equal(r.violation.code, 'OUT_OF_SCOPE');
  assert.deepEqual(r.violation.paths, ['src/b.mjs']);
});

test('a split may add siblings beside its allowlisted file', () => {
  const r = runChecks(
    facts({
      changed: ['src/big.mjs', 'src/big-sections.mjs'],
      insertions: 300, deletions: 300, primaryFileLines: 180,
      perFile: new Map([['src/big.mjs', { ins: 0, del: 300 }], ['src/big-sections.mjs', { ins: 300, del: 0 }]]),
    }),
    { category: 'LARGE_COMPONENT_SPLIT', allowlist: ['src/big.mjs'], original_lines: 620 },
    POLICY, { treeHashes: [] },
  );
  assert.equal(r.verdict, 'PASS');
});

test('deleting a test file is caught even inside the allowlist', () => {
  const r = runChecks(
    facts({ changed: ['src/a.test.mjs'], deleted: ['src/a.test.mjs'] }),
    { category: 'DEAD_CODE', allowlist: ['src/a.test.mjs'] }, POLICY, { treeHashes: [] },
  );
  assert.equal(r.violation.code, 'TEST_WEAKENED');
});

test('adding a skip marker to a test is caught', () => {
  const r = runChecks(
    facts({
      changed: ['src/a.test.mjs'],
      perFile: new Map([['src/a.test.mjs', { ins: 1, del: 0 }]]),
      addedByFile: { 'src/a.test.mjs': ['  it.skip("flaky", () => {})'] },
      insertions: 1, deletions: 0,
    }),
    { category: 'DEAD_CODE', allowlist: ['src/a.test.mjs'] }, POLICY, { treeHashes: [] },
  );
  assert.equal(r.violation.code, 'TEST_WEAKENED');
});

test('a tree seen earlier in the run is thrash', () => {
  const tree = 'c'.repeat(40);
  const r = runChecks(facts({ prospectiveTree: tree }), PACKET, POLICY, { treeHashes: [tree] });
  assert.equal(r.violation.code, 'THRASH_REVERT');
});

test('size caps fire before the category rule', () => {
  const r = runChecks(facts({ insertions: 5000, deletions: 0 }), PACKET, { max_files_per_candidate: 8, max_changed_lines: 600 }, { treeHashes: [] });
  assert.equal(r.violation.code, 'TOO_LARGE');
});

test('a split gets a line budget derived from the file it is splitting', () => {
  // Moving N lines out of one file and into another touches ~2N lines, so a flat
  // cap makes any file over ~300 lines unsplittable — exactly the files this
  // loop exists to split.
  const policy = { max_changed_lines: 600 };
  assert.equal(changedLineBudget({ category: 'DEAD_CODE' }, policy), 600);
  assert.equal(changedLineBudget({ category: 'LARGE_COMPONENT_SPLIT', original_lines: 620 }, policy), 1550);
  // Never smaller than the configured cap, even for a file just over the threshold.
  assert.equal(changedLineBudget({ category: 'LARGE_COMPONENT_SPLIT', original_lines: 520 }, policy), 1300);
  assert.equal(changedLineBudget({ category: 'LARGE_COMPONENT_SPLIT', original_lines: 0 }, policy), 600);
});

// The preamble carries rules the gate cannot enforce, so its wording is part of
// the contract. These assert the two bounds that a live run showed the model
// needs, without which it reasons correctly to a useless conclusion.
test('the preamble bounds "observable" so dead code can be removed at all', async () => {
  const { preamble } = await import('../src/prompts.mjs');
  // Normalise wrapping: the prompt is hard-wrapped, so phrases span newlines.
  const p = preamble('codex').replace(/\s+/g, ' ');
  // Observed live: a model refused a correct deletion because removing a file
  // "changes import('./that-file') from fulfillment to a resolution error" —
  // true of every deletion, and therefore a rule with no bound.
  assert.match(p, /hypothetical new caller/);
  assert.match(p, /reachable from an entrypoint that exists in this repository TODAY/i);
  // The exact shapes three live refusals produced, named so they cannot recur:
  // an unimported module's existence, and a stack trace observed through an
  // accessor the objector would have to install first.
  assert.match(p, /module that nothing in the repository imports/);
  assert.match(p, /shape of a stack trace/);
  assert.match(p, /internal helper frame/);
  // The bound must not read as permission to delete carelessly.
  assert.match(p, /does not soften the evidence requirements/);
});

test('phase routing restricts skills and reserves git ownership for the loop', async () => {
  const { preamble } = await import('../src/prompts.mjs');
  for (const provider of ['claude', 'codex']) {
    const p = preamble(provider);
    assert.match(p, /Invoke ONLY the skills named in the SKILLS line/);
    assert.match(p, /Do not invoke skills that are not listed for your phase/);
    assert.match(p, /Do not restructure the project or rewrite git history/);
    assert.match(p, /The loop owns git/);
  }
});

// doctor probes REQUIRED_SKILLS and demands every name; routing decides which
// name each phase writes into its prompt. If the two drift, doctor either passes
// a catalog that is missing a skill some phase needs, or fails a complete one.
test('doctor probes exactly the skills routing can reach', async () => {
  const { ROUTING, REQUIRED_SKILLS, auditPrompt, deepCheckPrompt, newNonce } = await import('../src/prompts.mjs');
  const routed = new Set(Object.values(ROUTING).flat());
  for (const skill of routed) assert.ok(REQUIRED_SKILLS.includes(skill), `routed but unprobed: ${skill}`);

  // sharpen-dedupe is the one skill no phase table names: it is added only for a
  // DEDUPLICATION candidate, so a set built from ROUTING alone would miss it.
  assert.ok(!routed.has('sharpen-dedupe'));
  assert.ok(REQUIRED_SKILLS.includes('sharpen-dedupe'));
  const nonce = newNonce();
  const dedup = deepCheckPrompt({
    provider: 'claude', nonce, candidate: { category: 'DEDUPLICATION', candidate_id: 'c', problem: 'p' },
    repoFacts: {}, areaSkills: '', commands: [],
  });
  assert.ok(dedup.includes('/sharpen-dedupe'), 'a DEDUPLICATION candidate must route sharpen-dedupe');
  assert.equal(REQUIRED_SKILLS.length, new Set(REQUIRED_SKILLS).size, 'no duplicate names');

  // And every probed name must be reachable, or doctor blocks on a skill the
  // loop would never invoke.
  const audit = auditPrompt({ provider: 'claude', nonce, incremental: false, repoFacts: {}, seen: { done: [], skipped: [] }, violated: [], cycle: 1 });
  assert.ok(audit.includes('/sharpen-clarify'), 'AUDIT must route sharpen-clarify');
});

test('declared deletions are checked against the allowlist like any other change', async () => {
  const { enforceExecutionVerdict } = await import('../src/schemas.mjs');
  const packet = { category: 'DEAD_CODE', allowlist: ['src/dead.mjs', 'src/index.mjs'] };
  const base = { verdict: 'PASS', hunks: [], scope_expansion_required: false, changed_files: [] };

  // Declaring a deletion inside the allowlist is the sanctioned way to remove a
  // file: no provider is given a deletion primitive, so this is the mechanism.
  assert.equal(enforceExecutionVerdict({ ...base, deleted_files: ['src/dead.mjs'] }, packet).verdict, 'PASS');

  // Declaring one outside it is a scope violation, exactly as editing it would be.
  const bad = enforceExecutionVerdict({ ...base, deleted_files: ['src/other.mjs'] }, packet);
  assert.equal(bad.verdict, 'FAIL');
  assert.match(bad.reasons.join(' '), /outside the allowlist/);
});

test('the execute prompt tells the model how to delete instead of letting it give up', async () => {
  const { executePrompt } = await import('../src/prompts.mjs');
  // Observed live: claude reported FAIL with "my available tool set contains no
  // file-deletion primitive", which made DEAD_CODE and COMPATIBILITY_REMOVAL
  // impossible for it — two of the four categories.
  const p = executePrompt({
    provider: 'claude', nonce: 'n', areaSkills: '-',
    packet: { candidate_id: 'C-1', category: 'DEAD_CODE', allowlist: ['a.mjs'], contracts: [], stop_conditions: [], minimal_change: 'x' },
  }).replace(/\s+/g, ' ');
  assert.match(p, /list its path in `deleted_files`/);
  assert.match(p, /do not report FAIL because you lack a deletion primitive/);
  assert.match(p, /Do not blank a file's contents as a substitute/);
});

test('write prompts state that build output is not a boundary violation', async () => {
  // Observed live: a characterization slice returned FAIL because `go build` —
  // a command the packet REQUIRED it to run — overwrote an untracked binary,
  // and the model concluded "the hard boundary was violated". Correct reasoning
  // from an unbounded rule, reaching a conclusion that makes the task
  // impossible: you cannot run a build without touching build output.
  const { characterizationPrompt, executePrompt } = await import('../src/prompts.mjs');
  const packet = {
    candidate_id: 'C-1', category: 'DEAD_CODE', allowlist: ['app/api/x.go'],
    contracts: [], stop_conditions: [], characterization_files: ['app/api/x_test.go'],
    minimal_change: 'remove it',
  };
  for (const [name, p] of Object.entries({
    characterization: characterizationPrompt({ provider: 'codex', nonce: 'n', packet, areaSkills: '-' }),
    execute: executePrompt({ provider: 'codex', nonce: 'n', packet, areaSkills: '-' }),
  })) {
    const flat = p.replace(/\s+/g, ' ');
    assert.match(flat, /[Bb]uild output is NOT a boundary violation|BUILD OUTPUT IS NOT A BOUNDARY VIOLATION/, name);
    assert.match(flat, /do not report FAIL|Do NOT report FAIL/i, `${name}: must say not to fail over it`);
    assert.match(flat, /orchestrator removes such files/i, `${name}: must say who cleans up`);
  }
});

// ---------------------------------------------------------------- --target

// The live log labels each step "n/N" from this constant. A stale N reads as
// "all N passed" on a run that stopped early.
test('the check count matches the checks a clean run actually emits', () => {
  const r = runChecks(facts(), PACKET, POLICY, { treeHashes: [] });
  assert.equal(r.checks.length, GATE_CHECK_COUNT);
});

// rank() and the audit prompt both work from the model's own account of which
// files a candidate touches. This is the only check that reads the change set
// that actually landed on disk against what the operator asked for.
test('a change that lands entirely outside the target is rejected', () => {
  const r = runChecks(
    facts({ changed: ['src/a.mjs'] }),
    PACKET, POLICY, { treeHashes: [] }, ['app/web'],
  );
  assert.equal(r.verdict, 'VIOLATION');
  assert.equal(r.violation.code, 'TARGET_SCOPE_EMPTY');
});

test('one changed path inside the target carries the rest', () => {
  const packet = { category: 'DEAD_CODE', allowlist: ['app/web/a.mjs', 'src/caller.mjs'] };
  const r = runChecks(
    facts({
      changed: ['app/web/a.mjs', 'src/caller.mjs'],
      perFile: new Map([['app/web/a.mjs', { ins: 0, del: 25 }], ['src/caller.mjs', { ins: 1, del: 5 }]]),
    }),
    packet, POLICY, { treeHashes: [] }, ['app/web'],
  );
  assert.equal(r.verdict, 'PASS', 'editing the caller outside the target is the expected case');
});

// An unscoped run must reach the same verdict it reached before this step
// existed, and must still emit GATE_CHECK_COUNT rows so the labels stay honest.
test('an unscoped run passes the target step without restricting anything', () => {
  const r = runChecks(facts(), PACKET, POLICY, { treeHashes: [] }, []);
  assert.equal(r.verdict, 'PASS');
  const step = r.checks.find((c) => c.id === 'TARGET_SCOPE');
  assert.ok(step?.ok);
  assert.match(step.detail, /no target restriction/);
});
