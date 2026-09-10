// End-to-end check of the containment story WITHOUT any provider:
// a real worktree, a real out-of-scope edit, the real gate, the real rollback.
import { test, before, after } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import os from 'node:os';
import { spawnSync } from 'node:child_process';
import * as G from '../src/git.mjs';
import { ensureRefactorDir } from '../src/state.mjs';
import { collectFacts, runChecks, rollbackCandidate } from '../src/gate.mjs';

const here = import.meta.dirname;
let repo, wt, baseOid, sourceFp;

before(() => {
  repo = fs.mkdtempSync(path.join(os.tmpdir(), 'rl-src-'));
  fs.rmSync(repo, { recursive: true, force: true });
  const r = spawnSync('node', [path.join(here, '..', 'fixtures', 'make-fixture.mjs'), repo], { encoding: 'utf8' });
  assert.equal(r.status, 0, r.stderr);
  baseOid = G.headOid(repo);
  sourceFp = G.sourceFingerprint(repo);
  wt = G.worktreePath(repo, 'test-run', path.join(os.tmpdir(), 'rl-wt'));
  G.worktreeAdd(repo, wt, baseOid);
});

after(() => {
  if (repo && wt) G.worktreeRemove(repo, wt);
  if (repo) fs.rmSync(repo, { recursive: true, force: true });
});

const POLICY = { max_files_per_candidate: 8, max_changed_lines: 600 };

function gate(packet) {
  // collectFacts stages the change set itself, so the numbers include new files.
  const facts = collectFacts({ wt, repoRoot: repo, preOid: G.headOid(wt), baseOid, sourceFp, packet });
  facts.prospectiveTree = G.writeTree(wt);
  return { facts, result: runChecks(facts, packet, POLICY, { treeHashes: [] }) };
}

test('the worktree is a real, detached checkout of the base commit', () => {
  assert.equal(G.headOid(wt), baseOid);
  assert.ok(fs.existsSync(path.join(wt, 'src/index.mjs')));
  assert.equal(G.currentBranch(wt), null, 'must be detached');
});

test('a fresh worktree presents an empty change set', () => {
  assert.deepEqual(G.changedPaths(wt), []);
  assert.equal(G.statusPorcelain(wt).trim(), '');
});

test('an in-scope deletion passes the gate', () => {
  fs.rmSync(path.join(wt, 'src/legacy-parser.mjs'));
  const { facts, result } = gate({ category: 'DEAD_CODE', allowlist: ['src/legacy-parser.mjs'] });
  assert.deepEqual(facts.changed, ['src/legacy-parser.mjs']);
  assert.equal(result.verdict, 'PASS', JSON.stringify(result.violation));
  const back = rollbackCandidate(wt, baseOid);
  assert.ok(back.clean);
});

test('an out-of-scope edit is caught at the allowlist check and rolled back exactly', () => {
  fs.rmSync(path.join(wt, 'src/legacy-parser.mjs'));
  fs.appendFileSync(path.join(wt, 'src/compat.mjs'), '\n// unrelated tidy-up\n');   // scope creep

  const { result } = gate({ category: 'DEAD_CODE', allowlist: ['src/legacy-parser.mjs'] });
  assert.equal(result.verdict, 'VIOLATION');
  assert.equal(result.violation.code, 'OUT_OF_SCOPE');
  assert.deepEqual(result.violation.paths, ['src/compat.mjs']);

  let saved = null;
  const back = rollbackCandidate(wt, baseOid, { savePatch: (p, files) => { saved = { p, files }; } });
  assert.ok(saved.p.length > 0, 'the rejected patch is kept as evidence');
  assert.ok(back.clean, `residue: ${back.residue}`);
  assert.equal(G.headOid(wt), baseOid);
  assert.equal(G.statusPorcelain(wt).trim(), '');
  assert.ok(fs.existsSync(path.join(wt, 'src/legacy-parser.mjs')), 'deleted file is restored');
  assert.ok(!fs.readFileSync(path.join(wt, 'src/compat.mjs'), 'utf8').includes('unrelated tidy-up'));
});

test('touching a lockfile is FORBIDDEN even when the allowlist names it', () => {
  fs.writeFileSync(path.join(wt, 'package-lock.json'), '{}\n');
  const { result } = gate({ category: 'DEAD_CODE', allowlist: ['package-lock.json'] });
  assert.equal(result.violation.code, 'FORBIDDEN_PATH');
  assert.ok(rollbackCandidate(wt, baseOid).clean);
});

test('rollback preserves hydrated gitignored build inputs', () => {
  const dep = path.join(wt, 'node_modules', 'left-pad');
  fs.mkdirSync(dep, { recursive: true });
  fs.writeFileSync(path.join(dep, 'index.js'), 'module.exports = 1;\n');
  fs.writeFileSync(path.join(wt, 'src/index.mjs'), '// clobbered\n');

  const back = rollbackCandidate(wt, baseOid);
  assert.ok(back.clean);
  assert.ok(fs.existsSync(path.join(dep, 'index.js')), 'clean -ffd must omit -x so hydration survives');
  assert.ok(fs.readFileSync(path.join(wt, 'src/index.mjs'), 'utf8').includes('renderPanel'));
});

test('the run directory self-ignores, so writing into it cannot dirty the repo', () => {
  const dir = ensureRefactorDir(repo);
  fs.mkdirSync(path.join(dir, 'runs', 'demo'), { recursive: true });
  fs.writeFileSync(path.join(dir, 'runs', 'demo', 'state.json'), '{}');
  assert.equal(G.statusPorcelain(repo).trim(), '', '.refactor/ must be invisible to git status');
});

test('the source repository is untouched by everything above', () => {
  assert.equal(G.headOid(repo), baseOid);
  assert.equal(G.statusPorcelain(repo).trim(), '');
  assert.equal(G.sourceFingerprint(repo), sourceFp);
});

test('a commit in the worktree is publishable into the source by update-ref alone', () => {
  fs.rmSync(path.join(wt, 'src/legacy-parser.mjs'));
  G.stageExact(wt, ['src/legacy-parser.mjs']);
  const msg = path.join(ensureRefactorDir(repo), 'msg.txt');   // run dir, outside W
  fs.writeFileSync(msg, 'refactor: remove unreferenced legacy parser\n\nRefactor-Fingerprint: deadbeef1234\n');
  const oid = G.commit(wt, msg);

  const ref = 'refs/heads/refactor/auto-test-run';
  G.updateRefCas(repo, ref, oid, null);
  assert.ok(G.refExists(repo, ref));
  assert.match(G.logOneline(repo, ref, 1), /remove unreferenced legacy parser/);
  assert.deepEqual(G.trailerFingerprints(repo, ref).slice(0, 1), ['deadbeef1234']);

  // and the user's checkout still has not moved
  assert.equal(G.headOid(repo), baseOid);
  assert.equal(G.statusPorcelain(repo).trim(), '');

  // CAS refuses a stale expectation
  assert.throws(() => G.updateRefCas(repo, ref, baseOid, null), /update-ref/);

  G.checkoutDetached(wt, baseOid);
});

test('a split that creates new files is measured on its true net size', () => {
  // Regression: the change set was measured before staging, and
  // `git diff --numstat HEAD` cannot see untracked files. A split therefore
  // reported its source file's deletions with none of the insertions in the
  // files it created, so every successful split failed the net-size rule as a
  // "mass deletion". Observed live: +40/-200 in the original plus +89 and +107
  // in two new files was reported as net -160 instead of net +36.
  const orig = fs.readFileSync(path.join(wt, 'src/big-panel.mjs'), 'utf8').split('\n');
  const originalLines = orig.length - 1;

  const head = orig.slice(0, 40).join('\n') + '\n';
  fs.writeFileSync(path.join(wt, 'src/big-panel.mjs'), head);
  fs.writeFileSync(path.join(wt, 'src/panel-formatters.mjs'), orig.slice(40, 300).join('\n') + '\n');
  fs.writeFileSync(path.join(wt, 'src/panel-metrics.mjs'), orig.slice(300).join('\n') + '\n');

  const packet = { category: 'LARGE_COMPONENT_SPLIT', allowlist: ['src/big-panel.mjs'], original_lines: originalLines };
  const facts = collectFacts({ wt, repoRoot: repo, preOid: G.headOid(wt), baseOid, sourceFp, packet });

  assert.equal(facts.changed.length, 3, 'all three files are in the change set');
  assert.ok(facts.insertions > 300, `new files must contribute insertions, got ${facts.insertions}`);
  const net = facts.insertions - facts.deletions;
  assert.ok(Math.abs(net) < 60, `a redistribution nets near zero, got ${net}`);
  assert.ok(facts.primaryFileLines < 500, 'the split achieved its stated goal');

  facts.prospectiveTree = G.writeTree(wt);
  const r = runChecks(facts, packet, POLICY, { treeHashes: [] });
  assert.equal(r.verdict, 'PASS', JSON.stringify(r.violation));

  assert.ok(rollbackCandidate(wt, baseOid).clean);
});

test('a split that rewrites rather than redistributes is still rejected', () => {
  fs.writeFileSync(path.join(wt, 'src/big-panel.mjs'), 'export function renderPanel() { return {}; }\n');
  const packet = { category: 'LARGE_COMPONENT_SPLIT', allowlist: ['src/big-panel.mjs'], original_lines: 620 };
  const facts = collectFacts({ wt, repoRoot: repo, preOid: G.headOid(wt), baseOid, sourceFp, packet });
  facts.prospectiveTree = G.writeTree(wt);
  const r = runChecks(facts, packet, POLICY, { treeHashes: [] });
  assert.equal(r.violation.code, 'NET_SIZE_RULE');
  assert.ok(rollbackCandidate(wt, baseOid).clean);
});

// ---------------------------------------------------------------- paths a live run has not reached
//
// Five runs never triggered these, so they are exercised deterministically here
// against a real worktree rather than left as untested branches.

test('a declared deletion inside the allowlist is applied by the orchestrator', async () => {
  const { applyDeclaredDeletions } = await import('../src/loop.mjs');
  const packet = { allowlist: ['src/legacy-parser.mjs'] };
  const ctx = { wt, log: { info() {}, fail() {} } };

  const removed = applyDeclaredDeletions(ctx, packet, ['src/legacy-parser.mjs']);
  assert.deepEqual(removed, ['src/legacy-parser.mjs']);
  assert.ok(!fs.existsSync(path.join(wt, 'src/legacy-parser.mjs')));

  // Idempotent: a provider whose sandbox can delete may have done it already.
  assert.deepEqual(applyDeclaredDeletions(ctx, packet, ['src/legacy-parser.mjs']), []);
  assert.ok(rollbackCandidate(wt, baseOid).clean);
});

test('a declared deletion outside the allowlist is refused and deletes nothing', async () => {
  const { applyDeclaredDeletions } = await import('../src/loop.mjs');
  const failures = [];
  const ctx = { wt, log: { info() {}, fail: (m) => failures.push(m) } };

  const r = applyDeclaredDeletions(ctx, { allowlist: ['src/legacy-parser.mjs'] }, ['src/plugin-x.mjs']);
  assert.equal(r, null, 'must signal a scope violation');
  assert.match(failures.join(' '), /outside the allowlist/);
  assert.ok(fs.existsSync(path.join(wt, 'src/plugin-x.mjs')), 'the file must survive a refused declaration');
});

test('weakening a real test file is caught by the real gate', () => {
  const f = path.join(wt, 'test/contract.test.mjs');
  fs.writeFileSync(f, fs.readFileSync(f, 'utf8').replace("test('status labels are stable'", "test.skip('status labels are stable'"));

  const packet = { category: 'DEAD_CODE', allowlist: ['test/contract.test.mjs'] };
  const facts = collectFacts({ wt, repoRoot: repo, preOid: G.headOid(wt), baseOid, sourceFp, packet });
  facts.prospectiveTree = G.writeTree(wt);
  const r = runChecks(facts, packet, POLICY, { treeHashes: [] });

  assert.equal(r.violation.code, 'TEST_WEAKENED');
  assert.match(r.violation.detail, /skip\/only marker/);
  assert.ok(rollbackCandidate(wt, baseOid).clean);
});

test('mutating the source repository mid-candidate halts rather than committing', () => {
  // The gate re-checks the source fingerprint on every candidate. This is what
  // makes "do not edit the repo while a run is in progress" enforceable rather
  // than merely advised.
  fs.appendFileSync(path.join(wt, 'src/compat.mjs'), '\n// in-scope edit\n');
  const stale = 'a'.repeat(40) + ':' + 'b'.repeat(40);   // fingerprint from before the human touched it

  const packet = { category: 'DEAD_CODE', allowlist: ['src/compat.mjs'] };
  const facts = collectFacts({ wt, repoRoot: repo, preOid: G.headOid(wt), baseOid, sourceFp: stale, packet });
  const r = runChecks(facts, packet, POLICY, { treeHashes: [] });

  assert.equal(r.verdict, 'HALT', 'a moved source repository must halt, not merely fail the candidate');
  assert.equal(r.violation.code, 'SOURCE_MUTATED');
  assert.ok(r.violation.halt);
  assert.ok(rollbackCandidate(wt, baseOid).clean);
});

test('a compiled artifact left by a validation command is swept, not blamed on the agent', () => {
  // Observed live: a characterization slice was rejected for "touching
  // app/api/fixtureapi outside the test allowlist" — a 2.4 MB Mach-O binary
  // that `go build` produced. Sweeping only BINARY untracked files is safe
  // because the gate forbids every binary change outright, so such a file can
  // never be a legitimate refactoring output.
  const bin = path.join(wt, 'compiled-artifact');
  fs.writeFileSync(bin, Buffer.from([0x7f, 0x45, 0x4c, 0x46, 0x00, 0x01, 0x02, 0x00]));
  const src = path.join(wt, 'src/newly-split.mjs');
  fs.writeFileSync(src, 'export const x = 1;\n');

  assert.ok(G.changedPaths(wt).includes('compiled-artifact'), 'git sees it before the sweep');

  const swept = G.sweepBuildArtifacts(wt);
  assert.deepEqual(swept.map((s) => s.path), ['compiled-artifact']);
  assert.ok(!fs.existsSync(bin), 'the binary is gone');
  assert.ok(fs.existsSync(src), 'a new SOURCE file must survive — a split legitimately creates those');
  assert.deepEqual(G.changedPaths(wt), ['src/newly-split.mjs']);

  assert.deepEqual(G.sweepBuildArtifacts(wt), [], 'idempotent');
  assert.ok(rollbackCandidate(wt, baseOid).clean);
});

test('a JVM build report is swept only inside the build output it came from', () => {
  // Gradle and Maven write plain TEXT under build/ and target/ — test-results
  // XML, HTML reports — which the binary rule above cannot see, and which is
  // enough to kill every candidate as OUT_OF_SCOPE in a repository that has not
  // gitignored them. Widening the rule to text needs a different justification,
  // so it is narrowed to directories derived from the commands discovery
  // actually produced for that area.
  fs.mkdirSync(path.join(wt, 'build', 'test-results'), { recursive: true });
  const xml = path.join(wt, 'build', 'test-results', 'TEST-x.xml');
  fs.writeFileSync(xml, '<testsuite tests="3"/>\n');
  const src = path.join(wt, 'src/kept.mjs');
  fs.writeFileSync(src, 'export const y = 2;\n');
  // `build` must not swallow `buildsystem` — the trailing slash is what stops it.
  fs.mkdirSync(path.join(wt, 'buildsystem'), { recursive: true });
  const neighbour = path.join(wt, 'buildsystem', 'Keep.java');
  fs.writeFileSync(neighbour, 'class Keep {}\n');

  assert.deepEqual(G.sweepBuildArtifacts(wt), [], 'without outputDirs, text is left alone');
  assert.ok(fs.existsSync(xml));

  const swept = G.sweepBuildArtifacts(wt, { outputDirs: ['build'] });
  assert.deepEqual(swept.map((s) => s.path), ['build/test-results/TEST-x.xml']);
  assert.ok(!fs.existsSync(xml));
  // The one that matters: a source file the implementer may have legitimately
  // created is never swept, whatever outputDirs says.
  assert.ok(fs.existsSync(src), 'a new source file must survive');
  assert.ok(fs.existsSync(neighbour), 'a directory that merely starts with the same letters must survive');
  assert.deepEqual(G.sweepBuildArtifacts(wt, { outputDirs: ['build'] }), [], 'idempotent');

  fs.rmSync(src, { force: true });
  fs.rmSync(path.join(wt, 'buildsystem'), { recursive: true, force: true });
  fs.rmSync(path.join(wt, 'build'), { recursive: true, force: true });
  assert.ok(rollbackCandidate(wt, baseOid).clean);
});

test('the sweep leaves gitignored and empty files alone', () => {
  fs.mkdirSync(path.join(wt, 'node_modules', 'dep'), { recursive: true });
  const ignored = path.join(wt, 'node_modules', 'dep', 'bin.node');
  fs.writeFileSync(ignored, Buffer.from([0x00, 0x01]));      // binary but gitignored
  const empty = path.join(wt, 'empty-file');
  fs.writeFileSync(empty, '');

  const swept = G.sweepBuildArtifacts(wt).map((s) => s.path);
  assert.ok(!swept.includes('node_modules/dep/bin.node'), 'gitignored files are not in the change set to begin with');
  assert.ok(fs.existsSync(ignored));
  assert.ok(!swept.includes('empty-file'), 'an empty file has no NUL byte and is not a binary');

  fs.rmSync(empty, { force: true });
  fs.rmSync(path.join(wt, 'node_modules'), { recursive: true, force: true });
  assert.ok(rollbackCandidate(wt, baseOid).clean);
});

test('the review diff names untracked binaries but never dumps their bytes', () => {
  // Observed live: the gate swept a 2.4 MB `go build` artifact, then the
  // validation ladder ran `go build` and put it back, so the reviewer was shown
  // a diff containing the binary and returned BLOCKER / CANNOT_DETERMINE —
  // correctly, about an artifact that had nothing to do with the candidate.
  fs.writeFileSync(path.join(wt, 'src/extracted.mjs'), 'export const x = 1;\n');
  fs.writeFileSync(path.join(wt, 'compiled'), Buffer.concat([
    Buffer.from([0x7f, 0x45, 0x4c, 0x46, 0x00]), Buffer.alloc(4096, 0xab),
  ]));

  const diff = G.diffForReview(wt);
  assert.match(diff, /export const x = 1/, 'new source must be reviewable');
  assert.match(diff, /untracked binary file\(s\) omitted as build output/);
  assert.match(diff, /compiled/, 'and named, so the reviewer is not misled by silence');
  assert.ok(!diff.includes('\xab\xab\xab'), 'but its bytes must not reach the model');
  assert.ok(diff.length < 4096, `diff stayed small (${diff.length} bytes)`);

  fs.rmSync(path.join(wt, 'compiled'), { force: true });
  assert.ok(rollbackCandidate(wt, baseOid).clean);
});

test('sweeping twice is harmless, because the ladder recreates artifacts', () => {
  fs.writeFileSync(path.join(wt, 'out.bin'), Buffer.from([0x00, 0x01, 0x02]));
  assert.equal(G.sweepBuildArtifacts(wt).length, 1);
  assert.equal(G.sweepBuildArtifacts(wt).length, 0, 'second sweep finds nothing');
  fs.writeFileSync(path.join(wt, 'out.bin'), Buffer.from([0x00, 0x01, 0x02]));
  assert.equal(G.sweepBuildArtifacts(wt).length, 1, 'and catches it again when rebuilt');
  assert.ok(rollbackCandidate(wt, baseOid).clean);
});
