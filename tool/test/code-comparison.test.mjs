import { test } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { spawnSync } from 'node:child_process';
import { git } from '../src/git.mjs';
import { collectCodeComparison } from '../src/code-comparison.mjs';
import { buildReport, renderMarkdown } from '../src/report.mjs';

const cli = path.resolve(import.meta.dirname, '../bin/refactor-me.mjs');
function fixture(t) {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'comparison-'));
  t.after(() => fs.rmSync(dir, { recursive: true, force: true }));
  const repo = path.join(dir, 'repo');
  const runDir = path.join(dir, 'run');
  fs.mkdirSync(repo); fs.mkdirSync(runDir);
  git(repo, ['init', '-q']);
  git(repo, ['config', 'user.name', 'Test']);
  git(repo, ['config', 'user.email', 'test@example.invalid']);
  git(repo, ['config', 'commit.gpgsign', 'false']);
  const write = (name, text) => { fs.mkdirSync(path.dirname(path.join(repo, name)), { recursive: true }); fs.writeFileSync(path.join(repo, name), text); };
  const commit = () => { git(repo, ['add', '-A']); git(repo, ['commit', '-qm', 'fixture', '--allow-empty']); return git(repo, ['rev-parse', 'HEAD']).trim(); };
  const state = { repoRoot: repo, baseOid: null, publishedOid: null };
  return { repo, runDir, write, commit, state, collect: () => collectCodeComparison(state, runDir) };
}
const reportBase = { runId: 'example', status: 'DONE', reason: 'no eligible candidates remain', durationMinutes: 1,
  repoRoot: '/repo', baseCommit: 'a'.repeat(40), baseBranch: 'main', branch: null, worktree: '/gone',
  counters: { commits: 0 }, commits: [], skipped: [], providers: {}, validation: { describe: null, ran: [], notRun: [] } };

test('net published comparison covers add, delete, rename, mode, binary and successive commits', (t) => {
  const f = fixture(t);
  f.write('modify.txt', 'old\n'); f.write('remove.txt', 'remove me\n'); f.write('rename.txt', 'rename stays\n');
  f.write('executable', '#!/bin/sh\n'); f.write('binary.dat', Buffer.from([0, 1, 2]));
  f.state.baseOid = f.commit();
  f.write('test/characterization.mjs', 'import assert from "node:assert";\n');
  f.commit();
  f.write('modify.txt', 'intermediate\n'); f.commit();
  f.write('modify.txt', 'final\n'); fs.unlinkSync(path.join(f.repo, 'remove.txt'));
  fs.renameSync(path.join(f.repo, 'rename.txt'), path.join(f.repo, '이름 | `x`\t새\n파일.txt'));
  fs.chmodSync(path.join(f.repo, 'executable'), 0o755);
  f.write('binary.dat', Buffer.from([0, 3, 4]));
  f.state.publishedOid = f.commit();
  const comparison = f.collect();
  assert.equal(comparison.status, 'AVAILABLE', comparison.error);
  assert.equal(comparison.totals.files, 6);
  assert.deepEqual(comparison.totals, { files: 6, insertions: 2, deletions: 2, binary: 1 });
  assert.ok(comparison.files.some((file) => file.oldPath === 'rename.txt' && file.status === 'R100'));
  assert.ok(comparison.files.some((file) => file.oldMode === '100644' && file.newMode === '100755'));
  const patch = fs.readFileSync(path.join(f.runDir, 'changes.patch'), 'utf8');
  assert.match(patch, /test\/characterization.mjs/); assert.match(patch, /\+final/);
  assert.ok(!patch.includes('intermediate')); assert.match(patch, /Binary files/); assert.ok(!patch.includes('GIT binary patch'));
  f.write('rejected.txt', 'not committed\n');
  assert.deepEqual(f.collect(), comparison);
  git(f.repo, ['update-ref', 'refs/heads/result', f.state.baseOid]);
  f.write('modify.txt', 'later unrelated commit\n'); f.commit();
  assert.deepEqual(f.collect(), comparison);
  assert.equal(fs.readFileSync(path.join(f.runDir, 'changes.patch'), 'utf8'), patch);
  for (const language of ['en', 'ko']) {
    const md = renderMarkdown({ ...reportBase, codeComparison: comparison }, language);
    assert.ok(md.includes(comparison.preview)); assert.match(md, /&#124;/); assert.match(md, /&#92;n/);
    assert.match(md, /\[.*\]\(changes.patch\)/);
  }
});

test('collection works after a detached worktree is removed and includes characterization alone', (t) => {
  const f = fixture(t); f.write('source.mjs', 'export const n = 1;\n'); f.state.baseOid = f.commit();
  const wt = path.join(path.dirname(f.repo), 'worktree');
  git(f.repo, ['worktree', 'add', '--detach', wt, f.state.baseOid]);
  fs.writeFileSync(path.join(wt, 'characterization.test.mjs'), 'test evidence\n');
  git(wt, ['add', '.']); git(wt, ['commit', '-qm', 'characterization']);
  f.state.publishedOid = git(wt, ['rev-parse', 'HEAD']).trim();
  git(f.repo, ['update-ref', 'refs/heads/result', f.state.publishedOid]);
  // A rejected refactor never reaches a commit or the result ref.
  fs.writeFileSync(path.join(wt, 'source.mjs'), 'rejected\n');
  git(wt, ['restore', 'source.mjs']); git(f.repo, ['worktree', 'remove', wt]);
  const comparison = f.collect();
  assert.equal(comparison.status, 'AVAILABLE', comparison.error);
  assert.deepEqual(comparison.files.map((file) => file.path), ['characterization.test.mjs']);
});

test('no published commit and net-zero published commits explicitly report no changes', (t) => {
  const f = fixture(t);
  assert.equal(f.collect().status, 'NO_CHANGES'); assert.deepEqual(fs.readdirSync(f.runDir), []);
  f.write('x', 'original\n'); f.state.baseOid = f.commit();
  f.write('x', 'changed\n'); f.commit(); f.write('x', 'original\n'); f.state.publishedOid = f.commit();
  const c = f.collect(); assert.equal(c.status, 'NO_CHANGES'); assert.equal(c.preview, '');
  assert.equal(fs.statSync(path.join(f.runDir, 'changes.patch')).size, 0);
  assert.match(renderMarkdown({ ...reportBase, codeComparison: c }, 'ko'), /커밋된 코드 변경이 없습니다/);
});

for (const [name, content] of [
  ['line limit', 'line\n'.repeat(400)],
  ['byte limit', ('한글'.repeat(600) + '\n').repeat(40)],
  ['one oversized line', '한'.repeat(40000) + '\n'],
]) test(`preview respects complete lines and UTF-8 bytes: ${name}`, (t) => {
  const f = fixture(t); f.state.baseOid = f.commit(); f.write('large.txt', content); f.state.publishedOid = f.commit();
  const c = f.collect(); const patch = fs.readFileSync(path.join(f.runDir, 'changes.patch'), 'utf8');
  assert.equal(c.status, 'AVAILABLE', c.error); assert.equal(c.truncated, true);
  assert.ok(Buffer.byteLength(c.preview) <= 32768); assert.ok(c.preview.split('\n').length - 1 <= 200);
  assert.ok(c.preview.endsWith('\n')); assert.ok(patch.startsWith(c.preview)); assert.ok(!c.preview.includes('\ufffd'));
  assert.ok(patch.endsWith(content.trimEnd().split('\n').at(-1) + '\n'));
});

test('embedded Markdown fences do not escape the diff block; context is three lines', (t) => {
  const f = fixture(t); f.write('sample.md', Array.from({ length: 20 }, (_, i) => `${i}\n`).join(''));
  f.state.baseOid = f.commit(); f.write('sample.md', Array.from({ length: 20 }, (_, i) => i === 10 ? '``````\n' : `${i}\n`).join(''));
  f.state.publishedOid = f.commit(); const c = f.collect();
  assert.match(c.preview, /@@ -8,7 \+8,7 @@/);
  for (const language of ['en', 'ko']) assert.ok(renderMarkdown({ ...reportBase, codeComparison: c }, language).includes('```````diff\n' + c.preview + '```````'));
});

test('Git external diff and textconv are disabled', (t) => {
  const f = fixture(t); f.write('.gitattributes', '*.txt diff=custom\n'); f.write('x.txt', 'before\n'); f.state.baseOid = f.commit();
  f.write('x.txt', 'after\n'); f.state.publishedOid = f.commit();
  git(f.repo, ['config', 'diff.external', '/does-not-exist']);
  git(f.repo, ['config', 'diff.custom.textconv', '/does-not-exist']);
  const c = f.collect(); assert.equal(c.status, 'AVAILABLE', c.error); assert.match(c.preview, /\+after/);
});

test('missing objects, non-OID refs and patch storage failures become unavailable', (t) => {
  const f = fixture(t); f.write('x', 'before\n'); f.state.baseOid = f.commit(); f.write('x', 'after\n'); f.state.publishedOid = f.commit();
  for (const publishedOid of ['f'.repeat(40), 'HEAD']) {
    const c = collectCodeComparison({ ...f.state, publishedOid }, f.runDir);
    assert.equal(c.status, 'UNAVAILABLE'); assert.ok(c.error); assert.equal(c.patchFile, null);
  }
  fs.mkdirSync(path.join(f.runDir, 'changes.patch'));
  const c = f.collect(); assert.equal(c.status, 'UNAVAILABLE'); assert.deepEqual(fs.readdirSync(f.runDir), ['changes.patch']);
  assert.match(renderMarkdown({ ...reportBase, codeComparison: c }), /run result is unchanged/);
  const state = { ...f.state, runId: 'example', terminal: { status: 'DONE', reason: 'limit' },
    providers: {}, counters: { commits: 1 }, commits: [], seen: { skipped: [] } };
  const report = buildReport({ state, runDir: f.runDir, startedAt: Date.now() });
  assert.equal(report.status, 'DONE'); assert.equal(report.reason, 'limit'); assert.equal(report.codeComparison.status, 'UNAVAILABLE');
});

test('old JSON renders a missing-comparison notice', () => {
  assert.match(renderMarkdown(reportBase), /No code comparison was saved for this run/);
  assert.match(renderMarkdown(reportBase, 'ko'), /이 실행에는 코드 비교가 저장되지 않았습니다/);
});

test('CLI language views preserve saved JSON, Markdown and patch after repository changes', (t) => {
  const f = fixture(t); f.write('code', 'before\n'); f.state.baseOid = f.commit(); f.write('code', 'after\n'); f.state.publishedOid = f.commit();
  const comparison = f.collect();
  const report = { ...reportBase, codeComparison: comparison };
  fs.writeFileSync(path.join(f.runDir, 'report.json'), JSON.stringify(report));
  fs.writeFileSync(path.join(f.runDir, 'report.md'), renderMarkdown(report));
  fs.mkdirSync(path.join(f.repo, '.refactor'));
  fs.writeFileSync(path.join(f.repo, '.refactor/last-run.json'), JSON.stringify({ runDir: f.runDir }));
  f.write('code', 'unrelated dirty checkout\n');
  const snapshots = fs.readdirSync(f.runDir).map((name) => ({ name, data: fs.readFileSync(path.join(f.runDir, name)), mtime: fs.statSync(path.join(f.runDir, name)).mtimeMs }));
  for (const language of ['en', 'ko']) {
    for (const json of [false, true]) {
      const result = spawnSync(process.execPath, [cli, 'report', '--lang', language, ...(json ? ['--json'] : [])], {
        cwd: f.repo, encoding: 'utf8', env: { ...process.env, PATH: '/usr/bin:/bin' },
      });
      assert.equal(result.status, 0, result.stderr);
      assert.equal(result.stdout.trimEnd(), (json ? JSON.stringify(report) : renderMarkdown(report, language)).trimEnd());
    }
  }
  for (const snapshot of snapshots) {
    assert.deepEqual(fs.readFileSync(path.join(f.runDir, snapshot.name)), snapshot.data);
    assert.equal(fs.statSync(path.join(f.runDir, snapshot.name)).mtimeMs, snapshot.mtime);
  }
});
