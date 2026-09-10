// doctor.mjs — the preconditions this design refuses to assume.
//
// Each probe records into doctor.json; a BLOCKING failure means the run does not
// start. The two that matter most are the ones that would silently CORRUPT a run
// rather than fail it: whether project skills actually load under our flag set
// (if not, routing degrades to nothing and the loop still "works", badly), and
// whether the read-only tool set really denies writes.
//
// The live provider probes are deliberately folded into ONE model turn each, on
// a tiny schema, so that starting a run costs a rounding error rather than a
// real slice of the budget it is about to spend.

import fs from 'node:fs';
import path from 'node:path';
import os from 'node:os';
import { spawnSync } from 'node:child_process';
import * as G from './git.mjs';
import { callProvider } from './provider.mjs';
import { ensureRefactorDir } from './state.mjs';
import { VERSION } from './version.mjs';
import { REQUIRED_SKILLS } from './prompts.mjs';
import { sourceFiles, isUnderTarget } from './scope.mjs';

const PROBE_SCHEMA = {
  type: 'object',
  additionalProperties: false,
  required: ['answer', 'visible_skills', 'wrote_file'],
  properties: {
    answer: { type: 'integer' },
    visible_skills: { type: 'array', items: { type: 'string' } },
    wrote_file: { type: 'boolean' },
  },
};

const PROBE_BODY = (sigil) => `This is an automated capability probe. Answer briefly and precisely.

1. Set "answer" to exactly 7.
2. Set "visible_skills" to the subset of these skill names that are actually
   available to you in this session: ${REQUIRED_SKILLS.join(', ')}. List only
   names you can genuinely see. They are invoked as ${sigil}name. Do not list a
   name you cannot see, and do not guess.
3. Attempt exactly once to create a file named probe-writable.txt in the current
   working directory, containing the word probe. Set "wrote_file" to true only if
   the write actually succeeded. If the attempt was refused or no write tool is
   available to you, set it to false. Do not work around a refusal, and do not
   attempt any other write.

Emit one JSON object matching the provided schema and nothing else.`;

const ok = (id, detail, blocking = false) => ({ id, status: 'PASS', blocking, detail });
const warn = (id, detail) => ({ id, status: 'WARN', blocking: false, detail });
const fail = (id, detail, blocking = true, fix = null) => ({ id, status: 'FAIL', blocking, detail, fix });

// ---------------------------------------------------------------- local probes

function which(bin) {
  const r = spawnSync('/usr/bin/which', [bin], { encoding: 'utf8' });
  return r.status === 0 ? r.stdout.trim() : null;
}

function version(bin) {
  const r = spawnSync(bin, ['--version'], { encoding: 'utf8', timeout: 20_000 });
  return r.status === 0 ? (r.stdout || r.stderr).trim().split('\n')[0] : null;
}

function probeWorktree(repoRoot) {
  const tmp = path.join(os.tmpdir(), `rl-probe-${process.pid}-${Date.now()}`);
  const add = G.gitTry(repoRoot, ['worktree', 'add', '--detach', tmp, 'HEAD']);
  if (!add.ok) return fail('git-worktree', `git worktree add failed: ${add.stderr.split('\n')[0]}`, true);
  G.gitTry(repoRoot, ['worktree', 'remove', '--force', tmp]);
  G.gitTry(repoRoot, ['worktree', 'prune']);
  fs.rmSync(tmp, { recursive: true, force: true });
  return ok('git-worktree', 'worktree add/remove works', true);
}

/**
 * Skill catalogs are inspected, never repaired. A broken symlink in someone's
 * repository is their business; silently fixing it would be a change we never
 * agreed to make.
 */
function probeSkills(repoRoot) {
  const out = [];
  let total = 0;
  const walk = (dir, depth) => {
    if (depth > 3) return;
    let entries = [];
    try { entries = fs.readdirSync(dir, { withFileTypes: true }); } catch { return; }
    for (const e of entries) {
      if (!e.isDirectory() && !e.isSymbolicLink()) continue;
      if (['node_modules', '.git', 'dist', 'build'].includes(e.name)) continue;
      const abs = path.join(dir, e.name);
      if (e.name === '.agents' || e.name === '.claude') {
        const skills = path.join(abs, 'skills');
        if (!fs.existsSync(skills)) continue;
        let names = [];
        try { names = fs.readdirSync(skills, { withFileTypes: true }); } catch { continue; }
        const broken = [];
        for (const s of names) {
          const p = path.join(skills, s.name);
          if (!fs.existsSync(p)) { broken.push(path.relative(repoRoot, p)); continue; }
          if (e.name === '.agents' && fs.existsSync(path.join(p, 'SKILL.md'))) total += 1;
        }
        if (broken.length) out.push(warn('skill-symlinks', `broken skill links (not repaired): ${broken.join(', ')}`));
      } else if (!e.name.startsWith('.')) walk(abs, depth + 1);
    }
  };
  walk(repoRoot, 0);
  out.unshift(total > 40
    ? warn('skill-count', `${total} skills present; descriptions may be truncated by the provider's context budget (harmless here, because every skill is invoked by explicit sigil)`)
    : ok('skill-count', `${total} skill(s) in .agents/skills`));
  return out;
}

/**
 * Where each provider actually looks. codex reads `.agents/skills` at its cwd
 * root and never `.claude/skills`; claude reads `.claude/skills` and follows its
 * symlinks. Both also read a global catalog under the home directory, so a
 * repository with no local skills is not evidence of a missing installation —
 * which is why both roots are tried before calling a name missing.
 */
/**
 * Where each provider can actually load a skill from, which is NOT the same for
 * the two — measured, not assumed, by probing each CLI with this loop's own
 * isolation flags from an empty scratch repository holding no catalog:
 *
 *   claude  --setting-sources project      user-scope skills: NOT visible ([])
 *   codex   --ignore-user-config           user-scope skills: visible (all 5)
 *
 * So `~/.claude/skills` must not appear here. Listing it would let doctor report
 * a complete catalog for a claude session that loads none of it — the exact
 * silent degradation this file exists to prevent. `~/.agents/skills` stays,
 * because for codex a user-scope install genuinely works and is also immune to
 * the worktree gap below.
 *
 * The asymmetry looks like an oversight and is not. Do not "fix" it without
 * re-running that probe.
 */
export const SKILL_ROOTS = {
  codex: (repoRoot) => [path.join(repoRoot, '.agents', 'skills'), path.join(os.homedir(), '.agents', 'skills')],
  claude: (repoRoot) => [path.join(repoRoot, '.claude', 'skills')],
};

/**
 * Deterministic per-name accounting: does every skill the loop can route to
 * resolve to a readable SKILL.md under the paths this provider reads?
 * `existsSync` follows symlinks, so a `.claude/skills` entry pointing at a
 * deleted directory counts as missing — which is exactly what it is.
 */
function resolveSkills(provider, repoRoot) {
  const roots = SKILL_ROOTS[provider](repoRoot);
  const at = new Map();
  for (const name of REQUIRED_SKILLS) {
    for (const root of roots) {
      if (fs.existsSync(path.join(root, name, 'SKILL.md'))) { at.set(name, root); break; }
    }
  }
  return { at, missing: REQUIRED_SKILLS.filter((n) => !at.has(n)), roots };
}

/**
 * doctor probes the SOURCE repository; phases run in a detached worktree of the
 * base commit. A skill installed project-scoped and never committed — merely
 * untracked is enough, gitignored is the common way — resolves here and is
 * absent there, so the run loses that skill in every phase while every phase
 * still reports success. A committed skill is in the checkout by definition, so
 * the reportable case is exactly "repo-local but not tracked".
 *
 * BLOCKING, unlike most of this file. The failure is invisible from the outside:
 * routing degrades to nothing, no phase errors, and the report reads like a
 * clean run that simply found little. A warning in front of an unattended run
 * that then spends an hour is not a control.
 *
 * The remedy is to commit them. A user-scope install is NOT an alternative here:
 * per SKILL_ROOTS above, claude cannot load one at all.
 */
export function probeSkillsInWorktree(repoRoot, resolved) {
  const local = [...resolved.at.entries()].filter(([, root]) => root.startsWith(repoRoot));
  if (!local.length) return [];
  const tracked = new Set(G.gitTry(repoRoot, ['ls-files']).stdout.split('\n').filter(Boolean));
  const absent = local
    .map(([name, root]) => [name, path.relative(repoRoot, path.join(root, name, 'SKILL.md'))])
    .filter(([, rel]) => !tracked.has(rel))
    .map(([name]) => name);
  return absent.length
    ? [fail('skill-worktree',
      `${absent.join(', ')} resolve in this repository but are not committed; every phase runs in a worktree checkout of the base commit, so an uncommitted skill is absent from all of them while each phase still reports success`,
      true, 'commit the catalog: git add .agents .claude && git commit — a plain `git stash` will not do, these are untracked')]
    : [];
}

/**
 * Two signals, deliberately weighted differently.
 *
 * Path resolution is mechanical and per-name, so it is what "all eight are
 * installed" is decided on. The model's own listing is NOT: observed live on
 * claude 2.1.236 with all eight installed and loadable, the model listed one of
 * eight. Requiring all eight from a self-report therefore excludes a healthy
 * provider — the regression `doctor.test.mjs` exists to prevent. What the
 * listing IS good for is the failure it was added for: a session that can see
 * NO skill at all, which is what `--tools` without `Skill` looks like, and which
 * degrades every phase to nothing while every phase still reports success.
 *
 * Exported for tests: the four combinations are the whole logic.
 */
export function skillVisibility(provider, { missing, visible, roots, home }) {
  const n = REQUIRED_SKILLS.length;
  const where = (p) => p.replace(home, '~');
  const install = "install the catalog into the project: npx skills add soom-kang/sharpen-me --skill '*' --agent codex claude-code";
  const blind = provider === 'claude'
    ? `${install} — it must be project-scoped (claude runs with --setting-sources project, so ~/.claude/skills is never loaded), Skill must be in --tools, and the .claude/skills links must resolve`
    : `${install} — codex reads .agents/skills at the cwd root and never .claude/skills`;

  if (missing.length === 0) {
    return visible.length > 0
      ? ok(`${provider}-skills`, `all ${n} project skills resolve; the model listed ${visible.length} in-session`, true)
      : fail(`${provider}-skills`, `all ${n} skills resolve on disk but the model could see none of them, so routing degrades to nothing while every phase still appears to succeed`, false, blind);
  }
  const detail = `${missing.length} of ${n} skills do not resolve under the paths ${provider} reads (${roots.map(where).join(', ')}): ${missing.join(', ')}`;
  return visible.length > 0
    ? warn(`${provider}-skills`, `${detail} — the model did list ${visible.length}, so those may load from somewhere else; verify before trusting a phase routed to a missing name`)
    : fail(`${provider}-skills`, `${detail}; a phase routed to a missing skill degrades to nothing while every phase still appears to succeed`, false, blind);
}

// ---------------------------------------------------------------- live probes

/**
 * Probe one provider. Every failure here is reported loudly but is NOT blocking:
 * one unhealthy provider is exactly the situation dual-provider support exists
 * for, and blocking the run would defeat it. Only "no provider is healthy"
 * blocks, which `runDoctor` decides after all providers have been probed.
 */
async function probeProviderLive(provider, cfg, repoRoot, runDir, log, usageSink) {
  const sigil = provider === 'claude' ? '/' : '$';
  const scratch = fs.mkdtempSync(path.join(os.tmpdir(), `rl-probe-${provider}-`));
  // Probe from the real repository so project skills are in scope, but keep the
  // write target inside a scratch cwd... which we cannot do for skills. So probe
  // in the repo and clean up any file the model manages to create.
  const target = path.join(repoRoot, 'probe-writable.txt');
  fs.rmSync(target, { force: true });

  const res = await callProvider(provider, {
    phase: 'doctor', mode: 'read', cwd: repoRoot, body: PROBE_BODY(sigil),
    schema: PROBE_SCHEMA, runDir, timeoutMs: 180_000, attempt: 'primary', effort: 'low',
  }, cfg, null);
  // The probe is one real turn per provider on every run. Small at effort low,
  // but it is spend, and a total that quietly omits it is not the total.
  usageSink?.push({ provider, usage: res.usage, durationMs: res.durationMs, processes: res.processes, ok: res.ok });

  const wroteReally = fs.existsSync(target);
  fs.rmSync(target, { force: true });
  fs.rmSync(scratch, { recursive: true, force: true });

  const checks = [];
  if (!res.ok) {
    const fixes = {
      AUTH: provider === 'claude' ? 'run `claude auth` (or /login in an interactive session)' : 'run `codex login`',
      QUOTA: 'wait for the quota window to reset, or run with the other provider',
      TIMEOUT: 'the CLI did not respond within 180s; check network and CLI health',
    };
    checks.push(fail(`${provider}-live`, `${res.failure}: ${res.detail ?? 'no detail'}`, false, fixes[res.failure] ?? null));
    return checks;
  }

  checks.push(res.data?.answer === 7
    ? ok(`${provider}-structured-output`, 'structured output arrives where the adapter reads it', true)
    : fail(`${provider}-structured-output`, `expected answer 7, got ${JSON.stringify(res.data?.answer)}`, false));

  const seen = (res.data?.visible_skills ?? []).map((s) => s.replace(/^[/$]/, ''));
  const { missing, roots } = resolveSkills(provider, repoRoot);
  checks.push(skillVisibility(provider, {
    missing, roots, home: os.homedir(), visible: REQUIRED_SKILLS.filter((s) => seen.includes(s)),
  }));

  checks.push(!wroteReally
    ? ok(`${provider}-read-only`, `read mode denied the write (model reported wrote_file=${res.data?.wrote_file})`, true)
    : fail(`${provider}-read-only`, 'a read-only session created a file; the read-only guarantee does not hold', false));

  return checks;
}

// ---------------------------------------------------------------- entrypoint

/**
 * A target with no source in it. Cheap to detect and expensive to discover the
 * slow way: without this the run spends a full high-effort audit turn on an
 * empty inventory and reports "no candidates", which reads as "your code is
 * clean" rather than "you pointed me at the wrong directory".
 */
function probeTargets(repoRoot, targets) {
  if (!targets.length) return [];
  const tracked = G.gitTry(repoRoot, ['ls-files']);
  if (!tracked.ok) return [warn('target', 'could not list tracked files to size the target')];
  const inScope = sourceFiles(tracked.stdout.split('\n').filter(Boolean))
    .filter((f) => isUnderTarget(f, targets));
  if (inScope.length === 0) {
    return [fail('target', `no tracked source file lives under ${targets.join(', ')}`, true,
      'point --target at a directory that contains code tracked by git')];
  }
  return [ok('target', `${targets.join(', ')} — ${inScope.length} tracked source file(s) in scope`)];
}

export async function runDoctor({ repoRoot, cfg, runDir, providers, log, live = true, targets = [] }) {
  const checks = [];
  // Before asserting the tree is clean, make our own footprint invisible —
  // otherwise the run directory is the thing that fails the check.
  ensureRefactorDir(repoRoot);

  // Its own row rather than a detail on `platform`: the non-darwin branch of
  // that check is a warning with no detail to hang this on, and a version that
  // disappears on Linux is useless for attributing beta feedback.
  checks.push(ok('version', `refactor-me ${VERSION}`));

  const plat = process.platform === 'darwin'
    ? ok('platform', `${process.platform} ${os.release()}, node ${process.versions.node}`)
    : warn('platform', `${process.platform} is untested; refactor-me targets macOS`);
  checks.push(plat);

  checks.push(probeWorktree(repoRoot));
  checks.push(...probeTargets(repoRoot, targets));

  const dirty = G.statusPorcelain(repoRoot).trim();
  checks.push(dirty === ''
    ? ok('source-clean', 'working tree and index are clean', true)
    : fail('source-clean', `the working tree is dirty:\n${dirty.split('\n').slice(0, 10).join('\n')}`, true,
      'commit or stash your work first — refactor-me refuses to run alongside uncommitted changes'));

  const available = [];
  for (const p of providers) {
    const bin = cfg.agents?.[p]?.bin || p;
    const where = which(bin);
    if (!where) { checks.push(warn(`${p}-cli`, `${bin} is not on PATH`)); continue; }
    available.push(p);
    checks.push(ok(`${p}-cli`, `${where} — ${version(bin) ?? 'version unknown'}`));
  }
  if (available.length === 0) {
    checks.push(fail('providers', 'neither claude nor codex is installed', true, 'install at least one provider CLI'));
    return finish(checks, runDir, [], []);
  }

  checks.push(...probeSkills(repoRoot));
  // Any provider's resolution answers this; the gap is about git, not the CLI.
  checks.push(...probeSkillsInWorktree(repoRoot, resolveSkills(available[0], repoRoot)));

  const healthy = [];
  const usageSink = [];
  const excluded = [];
  if (live) {
    for (const p of available) {
      log?.info(`probing ${p} (one small turn: structured output, skill visibility, write denial)`);
      const r = await probeProviderLive(p, cfg, repoRoot, runDir, log, usageSink);
      checks.push(...r);
      if (r.every((c) => c.status !== 'FAIL')) healthy.push(p);
      else { excluded.push(p); log?.warn(`${p} failed its probe and is excluded; the run proceeds on the other provider`); }
    }
    // A provider that fails its own probe is excluded, but the run may continue
    // on the other one — losing one provider to an expired token is exactly the
    // situation dual-provider support exists for.
    if (healthy.length === 0) {
      checks.push(fail('providers', 'no provider passed its probe', true,
        'see the per-provider failures above; usually `claude auth` or `codex login`'));
    }
  } else {
    healthy.push(...available);
  }

  return finish(checks, runDir, healthy, excluded, usageSink);
}

function finish(checks, runDir, healthy, excluded = [], usage = []) {
  const blocking = checks.filter((c) => c.status === 'FAIL' && c.blocking);
  const report = { at: new Date().toISOString(), version: VERSION, ok: blocking.length === 0, healthy, excluded, usage, checks };
  if (runDir) {
    fs.mkdirSync(runDir, { recursive: true });
    fs.writeFileSync(path.join(runDir, 'doctor.json'), JSON.stringify(report, null, 2));
  }
  return report;
}

export function renderDoctor(report) {
  const icon = { PASS: '[+]', WARN: '[!]', FAIL: '[x]' };
  const lines = report.checks.map((c) => {
    const head = `  ${icon[c.status]} ${c.id.padEnd(26)} ${c.detail}`;
    return c.fix ? `${head}\n      → ${c.fix}` : head;
  });
  const ring = report.healthy.join(', ') || 'none';
  const gone = (report.excluded ?? []).length ? `; excluded: ${report.excluded.join(', ')}` : '';
  return `doctor: ${report.ok ? 'OK' : 'BLOCKED'}  (healthy providers: ${ring}${gone})\n${lines.join('\n')}`;
}
