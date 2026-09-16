import { test } from 'node:test';
import assert from 'node:assert/strict';
import { renderDoctor, skillVisibility, probeSkillsInWorktree, runDoctor, SKILL_ROOTS } from '../src/doctor.mjs';
import * as G from '../src/git.mjs';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { spawnSync } from 'node:child_process';

// Regression: one unhealthy provider used to block the whole run. Observed live —
// `doctor: BLOCKED (healthy providers: codex)` while codex was perfectly fine,
// because a single `[x] claude-skills` was marked blocking. That defeats the one
// reason two providers are supported at all.
test('a report with one healthy provider is not blocked', () => {
  const report = {
    ok: true, healthy: ['codex'], excluded: ['claude'],
    checks: [
      { id: 'codex-skills', status: 'PASS', blocking: true, detail: 'all 8 project skills resolve' },
      { id: 'claude-skills', status: 'FAIL', blocking: false, detail: '2 of 8 skills do not resolve: sharpen-clarify, sharpen-review', fix: 'check .agents/skills' },
    ],
  };
  const out = renderDoctor(report);
  assert.match(out, /^doctor: OK/, 'one healthy provider is enough to start');
  assert.match(out, /healthy providers: codex; excluded: claude/, 'the excluded provider must be named');
  assert.match(out, /\[x\] claude-skills/, 'and its failure must still be visible, not hidden');
  assert.match(out, /→ check \.agents\/skills/, 'with its fix');
});

test('no healthy provider is blocked', () => {
  const out = renderDoctor({
    ok: false, healthy: [], excluded: ['codex', 'claude'],
    checks: [{ id: 'providers', status: 'FAIL', blocking: true, detail: 'no provider passed its probe', fix: 'claude auth login' }],
  });
  assert.match(out, /^doctor: BLOCKED/);
  assert.match(out, /healthy providers: none; excluded: codex, claude/);
});

test('a clean report names no exclusions', () => {
  const out = renderDoctor({ ok: true, healthy: ['codex', 'claude'], excluded: [], checks: [] });
  assert.match(out, /healthy providers: codex, claude\)/);
  assert.ok(!out.includes('excluded'));
});

// Observed live on claude 2.1.236: with all eight skills installed and loadable
// (this session had them), the probe listed ONE of eight. A doctor that demands
// eight from a self-report excludes that provider on every run, which is the
// regression the first test in this file exists to prevent. So per-name
// accounting comes from path resolution, and the listing only has to prove the
// session can see the catalog at all.
const args = (o) => ({ missing: [], visible: [], roots: ['/r/.claude/skills', '/home/.claude/skills'], home: '/home', ...o });

test('a complete catalog passes even when the model under-reports it', () => {
  const c = skillVisibility('claude', args({ visible: ['sharpen-assess'] }));
  assert.equal(c.status, 'PASS');
  assert.match(c.detail, /all 8 project skills resolve/);
  assert.match(c.detail, /listed 1 in-session/, 'the weaker signal is reported, not used as the verdict');
});

test('a complete catalog the session cannot see at all is a failure, not a warning', () => {
  const c = skillVisibility('claude', args({ visible: [] }));
  assert.equal(c.status, 'FAIL');
  assert.match(c.detail, /resolve on disk but the model could see none/);
  assert.match(c.fix, /Skill must be in --tools/, 'claude gets the --tools hint');
  assert.match(c.fix, /--setting-sources project/,
    'and the reason a user-scope install would not rescue it');
  assert.match(skillVisibility('codex', args({ visible: [] })).fix, /never \.claude\/skills/, 'codex gets its own');
});

test('a missing skill is named, and does not exclude a provider that can still see the rest', () => {
  const c = skillVisibility('codex', args({ missing: ['sharpen-dedupe', 'sharpen-brief'], visible: ['sharpen-clarify'] }));
  assert.equal(c.status, 'WARN', 'ambiguous: they may load from a path we do not know about');
  assert.match(c.detail, /2 of 8/);
  assert.match(c.detail, /sharpen-dedupe, sharpen-brief/, 'the gap must be named, not counted');
  assert.match(c.detail, /verify before trusting a phase routed to a missing name/);
});

test('nothing resolving and nothing visible is a plain uninstalled catalog', () => {
  const c = skillVisibility('codex', args({ missing: ['sharpen-clarify', 'sharpen-dedupe'], visible: [] }));
  assert.equal(c.status, 'FAIL');
  assert.match(c.detail, /sharpen-clarify, sharpen-dedupe/);
  assert.match(c.detail, /degrades to nothing/);
});

// `roots` is injected, so this pins the rendering, not the search. codex is the
// provider whose real root list still contains a home path — see SKILL_ROOTS.
test('the probed roots are reported with the home directory abbreviated', () => {
  const c = skillVisibility('codex', {
    missing: ['sharpen-clarify'], visible: ['sharpen-brief'],
    roots: ['/r/.agents/skills', '/home/.agents/skills'], home: '/home',
  });
  assert.match(c.detail, /\/r\/\.agents\/skills, ~\/\.agents\/skills/, 'an operator must be able to see where it looked');
});

// Measured, not assumed: probed from an empty scratch repo with this loop's own
// isolation flags, claude reported NO user-scope skill while codex reported all
// of them. Putting ~/.claude/skills back would make doctor certify a complete
// catalog for a claude session that loads none of it — which is precisely the
// silent degradation the skill probes exist to catch.
test('claude probes no home directory, codex still does', () => {
  const claude = SKILL_ROOTS.claude('/r');
  assert.deepEqual(claude, ['/r/.claude/skills'], 'a user-scope install is invisible under --setting-sources project');
  const codex = SKILL_ROOTS.codex('/r');
  assert.equal(codex.length, 2);
  assert.ok(codex[1].endsWith('/.agents/skills') && codex[1] !== '/r/.agents/skills',
    'codex genuinely loads a user-scope catalog, so doctor must look there');
});

// `realpathSync` matters on macOS, where mkdtemp hands back /var/... and the
// production repoRoot is realpath'd in git.mjs — `path.relative` needs both
// sides in the same namespace.
function newRepo(prefix) {
  const repo = fs.realpathSync(fs.mkdtempSync(path.join(os.tmpdir(), prefix)));
  const run = (...a) => assert.equal(
    spawnSync('git', a, { cwd: repo, encoding: 'utf8' }).status, 0, `git ${a.join(' ')}`);
  run('init', '-q');
  run('config', 'user.email', 't@t');
  run('config', 'user.name', 't');
  return { repo, run };
}

// The check reads a checkout now instead of guessing from the index, so every
// test supplies a real one.
function withWorktree(repo, fn) {
  const wt = path.join(os.tmpdir(), `rl-test-wt-${process.pid}-${Date.now()}`);
  G.worktreeAdd(repo, wt, 'HEAD');
  try { return fn(wt); } finally {
    G.worktreeRemove(repo, wt);
    fs.rmSync(wt, { recursive: true, force: true });
  }
}

const skillFile = (dir) => {
  fs.mkdirSync(dir, { recursive: true });
  fs.writeFileSync(path.join(dir, 'SKILL.md'), '---\nname: x\n---\n');
};

// doctor probes the source repository; phases run in a worktree checkout of the
// base commit. A project-scoped install that is gitignored resolves in the first
// and is absent from the second, so the run would lose every skill while doctor
// reported a complete catalog.
test('a repo-local skill that is not committed is reported, a committed one is not', () => {
  const { repo, run } = newRepo('rl-skills-');
  try {
    const root = path.join(repo, '.agents', 'skills');
    for (const name of ['sharpen-clarify', 'sharpen-review']) {
      fs.mkdirSync(path.join(root, name), { recursive: true });
      fs.writeFileSync(path.join(root, name, 'SKILL.md'), '---\nname: x\n---\n');
    }
    run('add', '.agents/skills/sharpen-clarify/SKILL.md');
    run('commit', '-qm', 'only one of them');

    const resolved = { at: new Map([['sharpen-clarify', root], ['sharpen-review', root]]) };
    withWorktree(repo, (wt) => {
    const out = probeSkillsInWorktree(repo, wt, [['codex', resolved]]);
    assert.equal(out.length, 1);
    assert.equal(out[0].status, 'FAIL');
    assert.equal(out[0].blocking, true,
      'a warning in front of an unattended hour-long run is not a control: routing degrades to '
      + 'nothing, no phase errors, and the report reads like a clean run that found little');
    assert.match(out[0].detail, /sharpen-review/);
    assert.doesNotMatch(out[0].detail, /sharpen-clarify/, 'a committed skill is in the checkout by definition');
    assert.match(out[0].fix, /git add/, 'the remedy is to commit, not to install elsewhere');
    assert.doesNotMatch(`${out[0].detail} ${out[0].fix}`, /globally/,
      'a user-scope install is not an alternative — claude cannot load one at all');

    // A root outside the repository is not this check's business: it has no
    // worktree gap, and for codex it is a supported install location.
    const global = { at: new Map([['sharpen-review', path.join(os.homedir(), '.agents', 'skills')]]) };
    assert.deepEqual(probeSkillsInWorktree(repo, wt, [['codex', global]]), []);
    });
  } finally {
    fs.rmSync(repo, { recursive: true, force: true });
  }
});

// THE regression this check was rewritten for. `npx skills add … --agent codex
// claude-code` puts the real files under .agents and a symlink at
// .claude/skills/<name>. git stores that link as ONE 120000 blob, so the old
// membership test for `.claude/skills/<name>/SKILL.md` could not match a path
// git will never emit — and blocked every `--provider claude` run against a
// fully committed catalog, including this repository's own.
test('a catalog reached through a committed .claude symlink is present in the checkout', () => {
  const { repo, run } = newRepo('rl-symlink-');
  try {
    const agents = path.join(repo, '.agents', 'skills');
    const claude = path.join(repo, '.claude', 'skills');
    fs.mkdirSync(claude, { recursive: true });
    for (const n of ['sharpen-clarify', 'sharpen-review']) {
      skillFile(path.join(agents, n));
      fs.symlinkSync(path.join('..', '..', '.agents', 'skills', n), path.join(claude, n));
    }
    run('add', '.agents', '.claude');
    run('commit', '-qm', 'catalog');

    // The premise, stated rather than assumed: the index cannot answer this.
    const ls = spawnSync('git', ['ls-files', '-s'], { cwd: repo, encoding: 'utf8' }).stdout;
    assert.match(ls, /^120000 .+\.claude\/skills\/sharpen-review$/m, 'the link is one blob');
    assert.ok(!ls.includes('.claude/skills/sharpen-review/SKILL.md'),
      'git never tracks a path through a symlink, so no index test can ever match it');

    withWorktree(repo, (wt) => {
      assert.deepEqual(probeSkillsInWorktree(repo, wt, [
        ['codex', { at: new Map([['sharpen-clarify', agents], ['sharpen-review', agents]]) }],
        ['claude', { at: new Map([['sharpen-clarify', claude], ['sharpen-review', claude]]) }],
      ]), [], 'link and target are both committed, so the checkout resolves the file');
    });
  } finally { fs.rmSync(repo, { recursive: true, force: true }); }
});

// The sub-case that makes "not committed" the wrong words: the link IS
// committed, and still resolves to nothing in the checkout.
test('a committed .claude symlink whose target is not committed is reported once', () => {
  const { repo, run } = newRepo('rl-dangling-');
  try {
    const agents = path.join(repo, '.agents', 'skills');
    const claude = path.join(repo, '.claude', 'skills');
    fs.mkdirSync(claude, { recursive: true });
    fs.writeFileSync(path.join(repo, '.gitignore'), '.agents/\n');
    skillFile(path.join(agents, 'sharpen-review'));
    fs.symlinkSync(path.join('..', '..', '.agents', 'skills', 'sharpen-review'),
      path.join(claude, 'sharpen-review'));
    run('add', '.gitignore', '.claude');
    run('commit', '-qm', 'links only');

    withWorktree(repo, (wt) => {
      const out = probeSkillsInWorktree(repo, wt, [
        ['codex', { at: new Map([['sharpen-review', agents]]) }],
        ['claude', { at: new Map([['sharpen-review', claude]]) }],
      ]);
      assert.equal(out.length, 1);
      assert.equal(out[0].status, 'FAIL');
      assert.equal(out[0].blocking, true);
      assert.equal(out[0].detail.match(/sharpen-review/g).length, 1,
        'one skill is one problem, however many providers cannot see it');
      assert.match(out[0].detail, /codex/);
      assert.match(out[0].detail, /claude/, 'the provider list is what makes the half-missing case diagnosable');
      assert.match(out[0].fix, /git add/);
    });
  } finally { fs.rmSync(repo, { recursive: true, force: true }); }
});

test('with no probe worktree the check reports that it did not measure', () => {
  const out = probeSkillsInWorktree('/r', null,
    [['claude', { at: new Map([['sharpen-review', '/r/.claude/skills']]) }]]);
  assert.equal(out.length, 1);
  assert.equal(out[0].status, 'WARN');
  assert.equal(out[0].blocking, false,
    'git-worktree already blocks the run; a second blocker only buries the cause');
  assert.match(out[0].detail, /not measured/);
});

// The probe worktree now outlives its own check, so its removal is a thing that
// can be forgotten. The early return on "no provider CLI" is the path where a
// `finally` is easiest to get wrong.
test('doctor removes its probe worktree even when no provider CLI is installed', async () => {
  const { repo, run } = newRepo('rl-doctor-');
  try {
    fs.writeFileSync(path.join(repo, 'a.txt'), 'x\n');
    run('add', 'a.txt');
    run('commit', '-qm', 'base');

    const report = await runDoctor({ repoRoot: repo, cfg: {}, runDir: null, providers: [], live: false, targets: [] });
    assert.equal(report.ok, false, 'no provider CLI is a blocking failure');
    const list = spawnSync('git', ['worktree', 'list'], { cwd: repo, encoding: 'utf8' }).stdout;
    assert.ok(!/rl-probe-/.test(list), `the probe worktree is still registered:\n${list}`);
  } finally { fs.rmSync(repo, { recursive: true, force: true }); }
});
