import { test } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { spawnSync } from 'node:child_process';
import { VERSION } from '../src/version.mjs';

const cli = path.resolve(import.meta.dirname, '../bin/refactor-me.mjs');
const installer = path.resolve(import.meta.dirname, '../install.mjs');
const run = (file, args, cwd) => spawnSync(process.execPath, [file, ...args], { cwd, encoding: 'utf8' });
const scratch = (t) => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'refactor-me-test-'));
  t.after(() => fs.rmSync(dir, { recursive: true, force: true }));
  return dir;
};
const initRepo = (dir) => {
  const result = spawnSync('git', ['init', '-q', dir], { encoding: 'utf8' });
  assert.equal(result.status, 0, result.stderr);
};

test('help and version work outside a repository under the new name', (t) => {
  const dir = scratch(t);
  for (const args of [['version', '--json'], ['--version', '--json'], ['run', '--version', '--json']]) {
    const result = run(cli, args, dir);
    assert.equal(result.status, 0, result.stderr);
    assert.equal(JSON.parse(result.stdout).name, 'refactor-me');
  }
  const help = run(cli, ['help'], dir);
  assert.equal(help.status, 0);
  assert.match(help.stdout, /--lang en\|ko/);
  assert.deepEqual(fs.readdirSync(dir), []);
});

test('invalid language arguments fail before creating any runtime files', (t) => {
  const dir = scratch(t);
  initRepo(dir);
  for (const args of [
    ['--lang'], ['--lang', '--json'], ['--lang', 'fr'], ['doctor', '--lang', 'ko'],
    ['clean', '--lang', 'ko'], ['version', '--lang', 'en'], ['--version', '--lang', 'ko'],
    ['help', '--lang', 'en'],
  ]) {
    const result = run(cli, args, dir);
    assert.equal(result.status, 2, args.join(' '));
    assert.match(result.stderr, /language|--lang/);
    assert.equal(result.stdout, '');
    assert.equal(fs.existsSync(path.join(dir, '.refactor')), false);
  }
});

test('report renders either language from JSON without rewriting stored evidence', (t) => {
  const dir = scratch(t);
  initRepo(dir);
  const runDir = path.join(dir, '.refactor', 'runs', 'old');
  fs.mkdirSync(runDir, { recursive: true });
  const report = {
    runId: 'old', status: 'NO_CHANGES', reason: 'no eligible candidates remain', durationMinutes: 4,
    repoRoot: dir, baseCommit: 'abcdef0123', baseBranch: 'main', branch: null, worktree: '/retained',
    counters: { cycles: 2, commits: 0, violations: 0 }, commits: [], skipped: [], providers: {},
    validation: { describe: 'GREEN 1', ran: ['node --test'], notRun: [] },
  };
  const jsonText = JSON.stringify(report, null, 2);
  fs.writeFileSync(path.join(runDir, 'report.json'), jsonText);
  fs.writeFileSync(path.join(runDir, 'report.md'), 'saved original report');
  fs.writeFileSync(path.join(dir, '.refactor', 'last-run.json'), JSON.stringify({ runDir }));
  for (const [args, pattern] of [[['report'], /## Committed changes/], [['report', '--lang', 'ko'], /## 커밋된 변경/]]) {
    const result = run(cli, args, dir);
    assert.equal(result.status, 0, result.stderr);
    assert.match(result.stdout, pattern);
    assert.doesNotMatch(result.stdout, /undefined/);
  }
  for (const lang of ['en', 'ko']) {
    const result = run(cli, ['report', '--json', '--lang', lang], dir);
    assert.equal(result.status, 0, result.stderr);
    assert.equal(result.stdout, jsonText);
  }
  assert.equal(fs.readFileSync(path.join(runDir, 'report.md'), 'utf8'), 'saved original report');
  assert.equal(fs.readFileSync(path.join(runDir, 'report.json'), 'utf8'), jsonText);
  assert.deepEqual(fs.readdirSync(runDir).sort(), ['report.json', 'report.md']);
});

test('report with missing JSON fails without replacing existing Markdown', (t) => {
  const dir = scratch(t);
  initRepo(dir);
  const runDir = path.join(dir, '.refactor');
  fs.mkdirSync(runDir);
  fs.writeFileSync(path.join(runDir, 'last-run.json'), JSON.stringify({ runDir }));
  fs.writeFileSync(path.join(runDir, 'report.md'), 'retain me');
  const result = run(cli, ['report', '--lang', 'ko'], dir);
  assert.equal(result.status, 2);
  assert.match(result.stderr, /report.json/);
  assert.equal(fs.readFileSync(path.join(runDir, 'report.md'), 'utf8'), 'retain me');
});

test('install and reinstall provide the new command and preserve data and custom files', (t) => {
  const dir = scratch(t);
  initRepo(dir);
  const first = run(installer, [dir], dir);
  assert.equal(first.status, 0, first.stderr);
  const dest = path.join(dir, '.refactor');
  const shim = path.join(dest, 'bin', 'refactor-me');
  const license = fs.readFileSync(path.resolve(import.meta.dirname, '../../LICENSE'), 'utf8');
  assert.equal(fs.readFileSync(path.join(dest, 'lib', 'LICENSE'), 'utf8'), license);
  assert.equal(spawnSync(shim, ['version'], { encoding: 'utf8' }).status, 0);
  const cfg = '{"workspace":{"branch_prefix":"custom/"}}\n';
  fs.writeFileSync(path.join(dest, 'config.json'), cfg);
  fs.mkdirSync(path.join(dest, 'runs', 'old'), { recursive: true });
  fs.writeFileSync(path.join(dest, 'runs', 'old', 'state.json'), '{"worktree":"/old/cache"}');
  fs.writeFileSync(path.join(dest, 'last-run.json'), '{"runId":"old"}');
  fs.writeFileSync(path.join(dest, 'bin', 'custom-command'), 'owned by user');
  const oldShim = path.join(dest, 'bin', 'refactorloop');
  fs.writeFileSync(oldShim, '#!/bin/sh\n# refactorloop 0.1.0 — installed 2026-09-06 from /old/tool\nexec node "$(dirname "$0")/../lib/bin/refactorloop.mjs" "$@"\n');
  fs.writeFileSync(path.join(dest, 'lib', 'bin', 'refactorloop.mjs'), '// old entrypoint');
  const updated = run(installer, [dir], dir);
  assert.equal(updated.status, 0, updated.stderr);
  assert.equal(fs.existsSync(oldShim), false);
  assert.equal(fs.existsSync(path.join(dest, 'lib', 'bin', 'refactorloop.mjs')), false);
  assert.equal(fs.readFileSync(path.join(dest, 'config.json'), 'utf8'), cfg);
  assert.equal(fs.readFileSync(path.join(dest, 'runs', 'old', 'state.json'), 'utf8'), '{"worktree":"/old/cache"}');
  assert.equal(fs.readFileSync(path.join(dest, 'last-run.json'), 'utf8'), '{"runId":"old"}');
  assert.equal(fs.readFileSync(path.join(dest, 'bin', 'custom-command'), 'utf8'), 'owned by user');
  const version = spawnSync(shim, ['version', '--json'], { encoding: 'utf8' });
  assert.equal(version.status, 0, version.stderr);
  assert.equal(JSON.parse(version.stdout).name, 'refactor-me');
  assert.equal(JSON.parse(version.stdout).version, VERSION);
  assert.equal(fs.readFileSync(path.join(dest, 'lib', 'LICENSE'), 'utf8'), license);
});

test('an incomplete distribution fails before replacing an existing installation', (t) => {
  const dir = scratch(t);
  const distribution = path.join(dir, 'distribution');
  const target = path.join(dir, 'target');
  initRepo(target);
  fs.cpSync(path.resolve(import.meta.dirname, '..'), path.join(distribution, 'tool'), { recursive: true });
  const lib = path.join(target, '.refactor', 'lib');
  fs.mkdirSync(lib, { recursive: true });
  fs.writeFileSync(path.join(lib, 'preserved.txt'), 'existing runtime');
  const result = run(path.join(distribution, 'tool', 'install.mjs'), [target], target);
  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /LICENSE/);
  assert.deepEqual(fs.readdirSync(lib), ['preserved.txt']);
  assert.equal(fs.readFileSync(path.join(lib, 'preserved.txt'), 'utf8'), 'existing runtime');
});

test('installer refuses an unrecognized old command before replacing files', (t) => {
  const dir = scratch(t);
  initRepo(dir);
  const bin = path.join(dir, '.refactor', 'bin');
  fs.mkdirSync(bin, { recursive: true });
  const oldShim = path.join(bin, 'refactorloop');
  fs.writeFileSync(oldShim, '#!/bin/sh\necho custom\n');
  const result = run(installer, [dir], dir);
  assert.equal(result.status, 2);
  assert.match(result.stderr, /unrecognized file/);
  assert.equal(fs.readFileSync(oldShim, 'utf8'), '#!/bin/sh\necho custom\n');
  assert.deepEqual(fs.readdirSync(path.dirname(bin)), ['bin']);
});
