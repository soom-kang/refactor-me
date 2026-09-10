import { test, before, after } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import os from 'node:os';
import { spawnSync } from 'node:child_process';
import {
  detectAreas, discoverArea, discoverCommands, extractSignature,
  signatureDelta, runBaseline, runLadder, areasForPaths, STATUS,
} from '../src/validate.mjs';

// ---------------------------------------------------------------- signatures

test('signature extraction picks up file:line and error codes', () => {
  const sig = extractSignature("src/broken.mjs:2: error TS2345: Argument of type 'unknown' ...");
  assert.deepEqual(sig, ['TS2345', 'src/broken.mjs:2']);
});

test('signature extraction handles go and node test failures', () => {
  assert.ok(extractSignature('--- FAIL: TestParse (0.00s)').includes('GOFAIL:TestParse'));
  assert.ok(extractSignature('✖ status labels are stable (1.2ms)').includes('FAIL:status labels are stable'));
});

test('a line-number shift in an already-failing file is tolerated', () => {
  const d = signatureDelta(['TS2345', 'src/broken.mjs:2'], ['TS2345', 'src/broken.mjs:9']);
  assert.ok(d.ok, 'moving code must not read as a regression');
  assert.deepEqual(d.drifted, ['src/broken.mjs:9']);
});

test('a complaint in a NEW file is a regression', () => {
  const d = signatureDelta(['TS2345', 'src/broken.mjs:2'], ['TS2345', 'src/broken.mjs:2', 'src/other.mjs:5']);
  assert.ok(!d.ok);
  assert.deepEqual(d.added, ['src/other.mjs:5']);
});

test('a new error code is a regression even in a known file', () => {
  const d = signatureDelta(['src/a.ts:1'], ['src/a.ts:1', 'TS7006']);
  assert.ok(!d.ok);
  assert.deepEqual(d.added, ['TS7006']);
});

test('fixing something is never a failure', () => {
  const d = signatureDelta(['TS2345', 'src/a.ts:1'], ['TS2345']);
  assert.ok(d.ok);
  assert.deepEqual(d.removed, ['src/a.ts:1']);
});

// ---------------------------------------------------------------- discovery

test('area scoping picks the deepest matching area', () => {
  const areas = ['.', 'app/api', 'app/web'];
  assert.deepEqual(areasForPaths(areas, ['app/web/src/x.tsx']).sort(), ['.', 'app/web']);
  assert.deepEqual(areasForPaths(areas, ['README.md']).sort(), ['.']);
});

test('a heavy test script is recorded as skipped, never executed', () => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'rl-disc-'));
  fs.writeFileSync(path.join(dir, 'package.json'), JSON.stringify({
    scripts: { test: 'playwright test', lint: 'eslint .', build: 'next build' },
  }));
  const cmds = discoverArea(dir, '.');
  assert.ok(cmds.some((c) => c.tier === 'SKIPPED' && c.name === 'test'));
  assert.ok(!cmds.some((c) => c.tier === 'T2'), 'playwright must not become a loop gate');
  assert.ok(cmds.some((c) => c.tier === 'T1' && c.name === 'lint'));
  fs.rmSync(dir, { recursive: true, force: true });
});

test('the package runner follows the lockfile', () => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'rl-run-'));
  fs.writeFileSync(path.join(dir, 'package.json'), JSON.stringify({ scripts: { build: 'tsc' } }));
  fs.writeFileSync(path.join(dir, 'pnpm-lock.yaml'), '');
  assert.equal(discoverArea(dir, '.')[0].argv[0], 'pnpm');
  fs.rmSync(dir, { recursive: true, force: true });
});

test('go areas never get -race or integration tags', () => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'rl-go-'));
  fs.writeFileSync(path.join(dir, 'go.mod'), 'module x\n');
  const argvs = discoverArea(dir, '.').map((c) => c.argv.join(' '));
  assert.ok(argvs.includes('go test -count=1 ./...'));
  assert.ok(!argvs.some((a) => a.includes('-race')));
  fs.rmSync(dir, { recursive: true, force: true });
});

test('a Makefile test target survives a pyproject that only contributes lint', () => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'rl-mk-'));
  fs.writeFileSync(path.join(dir, 'pyproject.toml'), '[tool.ruff]\nline-length = 100\n');
  fs.writeFileSync(path.join(dir, 'Makefile'),
    'lint:\n	ruff check .\n\ntest:\n	python -m unittest discover -s tests\n');
  const cmds = discoverArea(dir, '.');
  const byName = new Map(cmds.map((c) => [c.name, c]));
  assert.ok(byName.has('test'), 'the only test suite in the repo must not be hidden by a lint command');
  assert.deepEqual(byName.get('test').argv, ['make', 'test']);
  assert.equal(byName.get('test').tier, 'T2');
  assert.deepEqual(byName.get('lint').argv, ['ruff', 'check', '.'], 'the native lint still wins over the Makefile target');
  assert.equal(cmds.filter((c) => c.name === 'lint').length, 1);
  fs.rmSync(dir, { recursive: true, force: true });
});

test('a python tool that is only configured gets the bare binary, a declared one gets the runner', () => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'rl-py-'));
  fs.writeFileSync(path.join(dir, 'uv.lock'), '');
  fs.writeFileSync(path.join(dir, 'pyproject.toml'), '[tool.ruff]\nline-length = 100\n');
  assert.deepEqual(discoverArea(dir, '.')[0].argv, ['ruff', 'check', '.'],
    'a [tool.ruff] block is configuration, not an installed dependency');

  fs.writeFileSync(path.join(dir, 'pyproject.toml'),
    '[tool.ruff]\nline-length = 100\n\n[dependency-groups]\ndev = ["ruff>=0.4"]\n');
  assert.deepEqual(discoverArea(dir, '.')[0].argv, ['uv', 'run', '--', 'ruff', 'check', '.']);
  fs.rmSync(dir, { recursive: true, force: true });
});

// ------------------------------------------------- commands that cannot run

const MISSING = ['definitely-not-a-real-binary-xyz'];
const cmdSpec = (name, argv, tier = 'T1') => ({
  id: `.:${tier}:${name}`, area: '.', tier, name, argv, cwd: '.', timeoutMs: 30_000,
});

test('a binary that is not installed is UNRUNNABLE, never a RED differential gate', async () => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'rl-enoent-'));
  const b = await runBaseline([cmdSpec('lint', MISSING)], dir, null);
  const r = b.results[0];
  assert.equal(r.status, STATUS.UNRUNNABLE, 'a command that never ran is not a check that failed');
  assert.equal(r.spawnError, 'ENOENT');
  assert.equal(b.red, 0, 'an empty signature would compare equal to itself forever');
  assert.equal(b.unrunnable.length, 1);
  assert.ok(!b.usable);
  assert.match(b.describe, /not executable/);
  fs.rmSync(dir, { recursive: true, force: true });
});

test('an unrunnable baseline command is dropped from the ladder instead of passing it', async () => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'rl-enoent2-'));
  const cmds = [cmdSpec('lint', MISSING), cmdSpec('test', ['node', '-e', ''], 'T2')];
  const b = await runBaseline(cmds, dir, null);
  assert.equal(b.green, 1);
  assert.ok(b.usable, 'one real GREEN command is still enough to proceed');

  const l = await runLadder(b, cmds, dir, ['x.py'], ['.'], null);
  assert.ok(l.ok);
  assert.deepEqual(l.checks.map((c) => c.name), ['test'],
    'the missing binary must not be reported as a check that cleared the candidate');
  fs.rmSync(dir, { recursive: true, force: true });
});

test('a baseline RED check that goes unreadable is caught, not waved through', async () => {
  // The dangerous half of OPAQUE. Excluding it at BASELINE was only half the
  // job: a check can also stop being readable between baseline and candidate —
  // a dependency that no longer resolves, a toolchain download that fails. Then
  // signatureDelta compares against an empty set, finds nothing added, and
  // reports "no new signature", clearing a candidate on the strength of a
  // failure nobody could read.
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'rl-opaque-'));
  const flag = path.join(dir, 'unreadable');

  // RED while the flag is absent (a parseable complaint), OPAQUE once it exists.
  const script = `if (require('fs').existsSync(${JSON.stringify(flag)})) { console.error('could not resolve dependencies'); }`
    + ` else { console.error('src/broken.mjs:2: error TS2345: bad'); } process.exit(1)`;
  const cmds = [
    cmdSpec('typecheck', ['node', '-e', script]),
    cmdSpec('test', ['node', '-e', ''], 'T2'),
  ];

  const b = await runBaseline(cmds, dir, null);
  assert.equal(b.results[0].status, STATUS.RED, 'baseline fails in a way we can read');
  assert.ok(b.results[0].signature.length > 0);
  assert.ok(b.usable);

  fs.writeFileSync(flag, '');                       // now it fails for a different, unreadable reason
  const l = await runLadder(b, cmds, dir, ['src/x.mjs'], ['.'], null);
  assert.equal(l.ok, false, 'an unreadable failure must not clear a candidate');
  assert.equal(l.failed.name, 'typecheck');
  assert.equal(l.failed.failureKind, 'REGRESSION');
  assert.match(l.failed.why, /no recognisable output/);
  fs.rmSync(dir, { recursive: true, force: true });
});

// ---------------------------------------------------------------- live fixture

let repo;
before(() => {
  repo = path.join(os.tmpdir(), `rl-val-${process.pid}`);
  const r = spawnSync('node', [path.join(import.meta.dirname, '..', 'fixtures', 'make-fixture.mjs'), repo], { encoding: 'utf8' });
  assert.equal(r.status, 0, r.stderr);
});
after(() => { if (repo) fs.rmSync(repo, { recursive: true, force: true }); });

test('the fixture discovers all four commands in one area', () => {
  const d = discoverCommands(repo);
  assert.deepEqual(detectAreas(repo), ['.']);
  assert.deepEqual(d.commands.map((c) => `${c.tier}:${c.name}`).sort(),
    ['T1:lint', 'T1:typecheck', 'T2:test', 'T3:build']);
});

test('baseline is usable despite a permanently RED typecheck', async () => {
  const d = discoverCommands(repo);
  const b = await runBaseline(d.commands, repo, null);
  assert.equal(b.green, 3);
  assert.equal(b.red, 1);
  assert.ok(b.usable, 'one GREEN command is enough to proceed');
  const tc = b.results.find((r) => r.name === 'typecheck');
  assert.equal(tc.status, STATUS.RED);
  assert.deepEqual(tc.signature, ['TS2345', 'src/broken.mjs:2']);
});

test('the ladder passes when the RED command fails identically', async () => {
  const d = discoverCommands(repo);
  const b = await runBaseline(d.commands, repo, null);
  // a genuinely behaviour-preserving edit: shift broken.mjs down a line
  const f = path.join(repo, 'src/broken.mjs');
  fs.writeFileSync(f, '// a harmless leading comment\n' + fs.readFileSync(f, 'utf8'));
  const l = await runLadder(b, d.commands, repo, ['src/broken.mjs'], d.areas, null);
  assert.ok(l.ok, JSON.stringify(l.failed?.why));
  assert.ok(l.checks.find((c) => c.id.endsWith('typecheck')).why.includes('line-drift'));
  spawnSync('git', ['checkout', '--', 'src/broken.mjs'], { cwd: repo });
});

test('the ladder fails when a NEW file starts complaining', async () => {
  const d = discoverCommands(repo);
  const b = await runBaseline(d.commands, repo, null);
  fs.appendFileSync(path.join(repo, 'src/compat.mjs'), '\n// UNTYPED_MARKER introduced by a bad refactor\n');
  const l = await runLadder(b, d.commands, repo, ['src/compat.mjs'], d.areas, null);
  assert.ok(!l.ok);
  assert.equal(l.failed.failureKind, 'REGRESSION');
  assert.match(l.failed.why, /NEW signature/);
  spawnSync('git', ['checkout', '--', 'src/compat.mjs'], { cwd: repo });
});

test('the ladder fails when a GREEN command turns RED', async () => {
  const d = discoverCommands(repo);
  const b = await runBaseline(d.commands, repo, null);
  fs.appendFileSync(path.join(repo, 'src/status-a.mjs'), '\nvar sloppy = 1;\n');   // lint was GREEN
  const l = await runLadder(b, d.commands, repo, ['src/status-a.mjs'], d.areas, null);
  assert.ok(!l.ok);
  assert.match(l.failed.why, /was GREEN at baseline/);
  spawnSync('git', ['checkout', '--', 'src/status-a.mjs'], { cwd: repo });
});

// ---------------------------------------------------------------- external surface

test('a private package with no entrypoints reports NONE_DETECTED', async () => {
  const { externalSurface } = await import('../src/prompts.mjs');
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'rl-ext-'));
  fs.writeFileSync(path.join(dir, 'package.json'), JSON.stringify({ private: true, scripts: { test: 'node --test' } }));
  const r = externalSurface(dir);
  assert.equal(r.verdict, 'NONE_DETECTED');
  assert.deepEqual(r.reasons, []);
  fs.rmSync(dir, { recursive: true, force: true });
});

test('a published entrypoint or release workflow reports POSSIBLE', async () => {
  const { externalSurface } = await import('../src/prompts.mjs');
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'rl-ext2-'));
  fs.writeFileSync(path.join(dir, 'package.json'), JSON.stringify({ private: true, exports: './index.js' }));
  assert.equal(externalSurface(dir).verdict, 'POSSIBLE');

  fs.writeFileSync(path.join(dir, 'package.json'), JSON.stringify({ private: true }));
  fs.mkdirSync(path.join(dir, '.github', 'workflows'), { recursive: true });
  fs.writeFileSync(path.join(dir, '.github', 'workflows', 'r.yml'), 'jobs:\n  x:\n    steps:\n      - run: npm publish\n');
  const r = externalSurface(dir);
  assert.equal(r.verdict, 'POSSIBLE');
  assert.match(r.reasons.join(' '), /publishes an artifact/);
  fs.rmSync(dir, { recursive: true, force: true });
});

test('an importable go module path counts as an external surface', async () => {
  const { externalSurface } = await import('../src/prompts.mjs');
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'rl-ext3-'));
  fs.writeFileSync(path.join(dir, 'go.mod'), 'module github.com/acme/thing\n\ngo 1.22\n');
  assert.equal(externalSurface(dir).verdict, 'POSSIBLE');
  fs.writeFileSync(path.join(dir, 'go.mod'), 'module internalthing\n\ngo 1.22\n');
  assert.equal(externalSurface(dir).verdict, 'NONE_DETECTED');
  fs.rmSync(dir, { recursive: true, force: true });
});
