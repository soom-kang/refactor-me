#!/usr/bin/env node
// refactor-me — one command, unattended, behavior-preserving refactoring.
//
//   refactor-me [run]        run the loop (default)
//   refactor-me doctor       preconditions only, no refactoring
//   refactor-me report       print the last run's report
//   refactor-me clean        remove finished worktrees
//   refactor-me version      print the tool version and exit
//   refactor-me help

import fs from 'node:fs';
import path from 'node:path';
import { Logger } from '../src/log.mjs';
import * as G from '../src/git.mjs';
import * as S from '../src/state.mjs';
import { runLoop } from '../src/loop.mjs';
import { loadConfig } from '../src/config.mjs';
import { VERSION } from '../src/version.mjs';
import { runDoctor, renderDoctor } from '../src/doctor.mjs';
import { buildReport, renderSummary, renderMarkdown, validateLanguage } from '../src/report.mjs';
import { PROVIDERS } from '../src/provider.mjs';
import { normalizeTargets } from '../src/scope.mjs';

const EXIT = { OK: 0, ABORTED: 2, HALTED_UNSAFE: 4 };

function parseArgs(argv) {
  const out = { command: 'run', version: false, provider: null, fallback: undefined, json: false, live: true, forceQuotaAt: null, targets: [], language: 'en', languageSet: false };
  const rest = [];
  // `--target x --target y` accumulates. A missing value is caught here rather
  // than downstream: `--target --json` would otherwise consume the next flag as
  // a directory name and fail with a confusing "does not exist".
  const value = (i, name) => {
    const v = argv[i];
    if (v === undefined || v.startsWith('-')) { console.error(`${name} needs a directory`); process.exit(EXIT.ABORTED); }
    return v;
  };
  for (let i = 0; i < argv.length; i++) {
    const a = argv[i];
    if (a === '--target') out.targets.push(value(++i, '--target'));
    else if (a === '--lang') {
      const language = argv[++i];
      if (!language || language.startsWith('-')) throw new Error('--lang needs en or ko');
      out.language = validateLanguage(language);
      out.languageSet = true;
    }
    else if (a === '--provider') out.provider = argv[++i];
    else if (a === '--fallback') out.fallback = argv[++i];
    else if (a === '--json') out.json = true;
    else if (a === '--no-live-probe') out.live = false;
    else if (a === '--force-quota-at') out.forceQuotaAt = argv[++i];
    // A flag, not a command: the `rest[0]` assignment below overwrites
    // out.command, so `refactor-me run --version` would have started a real
    // refactoring run.
    else if (a === '--version') out.version = true;
    else if (a === '-h' || a === '--help') out.command = 'help';
    else if (!a.startsWith('-')) rest.push(a);
    else { console.error(`unknown option: ${a}`); process.exit(EXIT.ABORTED); }
  }
  if (rest.length) out.command = rest[0];
  if (out.languageSet && (out.version || !['run', 'report'].includes(out.command))) {
    throw new Error('--lang is supported only for run and report');
  }
  return out;
}

/**
 * Provider order. `--provider X` puts X first; `--fallback none` runs with one.
 * The ring itself decides when to switch — only on quota or auth.
 */
function providerOrder(cfg, args) {
  const primary = args.provider ?? cfg.agents.primary ?? 'codex';
  if (!PROVIDERS.includes(primary)) { console.error(`unknown provider: ${primary}`); process.exit(EXIT.ABORTED); }
  const fb = args.fallback === undefined ? (cfg.agents.fallback ?? null) : args.fallback;
  if (fb === 'none' || fb === null) return [primary];
  if (!PROVIDERS.includes(fb)) { console.error(`unknown fallback: ${fb}`); process.exit(EXIT.ABORTED); }
  return [...new Set([primary, fb])];
}

function resolveRepo() {
  const root = G.resolveRepoRoot(process.cwd());
  if (!root) {
    console.error('refactor-me must be run inside a git repository.');
    console.error('It works by committing into a detached worktree and publishing one local branch,');
    console.error('so a git repository is not an incidental requirement.');
    process.exit(EXIT.ABORTED);
  }
  return root;
}

const HELP = `refactor-me — unattended behavior-preserving refactoring

USAGE
  refactor-me [run] [options]     audit → implement → validate → review → commit, repeatedly
  refactor-me doctor [options]    check preconditions only; changes nothing
  refactor-me report              print the most recent run's report
  refactor-me clean               remove worktrees from finished runs
  refactor-me version             print the tool version and exit

OPTIONS
  --target <dir>            look for candidates only inside <dir>; repeatable
  --provider claude|codex   which agent leads (default: config, else codex)
  --fallback claude|codex|none
                            who takes over on quota or auth failure (default: the other one)
  --json                    machine-readable summary on stdout
  --lang en|ko              report and final summary language (run/report; default: en)
  --no-live-probe           skip the live provider probes in doctor (faster, less certain)

WHAT IT DOES
  Creates a detached git worktree outside the repository, refactors there, and
  publishes accepted commits one at a time to a local refactor/auto-* branch.
  Your working tree, index and HEAD are never touched.

SCOPING A RUN
  --target narrows what is SURVEYED, not what may be changed. The audit looks
  for candidates only inside the target directory, but a candidate rooted there
  may still edit its callers, re-exports and tests outside it — a change that
  stopped at a directory boundary would not compile. Reachability checks and the
  validation ladder stay repository-wide either way: narrowing what we look at
  never narrows what we verify.

WHAT IT WILL NOT DO
  Merge, push, deploy, install dependencies, change a manifest or lockfile,
  update a snapshot, weaken a test, or fix a bug. A run that changes nothing is
  a valid outcome.

EXIT CODES
  0  completed, or completed partially (a branch may exist either way)
  2  aborted before any write (dirty tree, no provider, no usable baseline)
  4  halted unsafe — an invariant broke; the worktree is preserved as evidence
`;

/**
 * Which build is this. `source` matters more than it looks: during the beta the
 * same person often has a development checkout (.../refactor-me/tool) and an
 * installed copy (.../.refactor/lib) on the machine, and "which one did you
 * run" is otherwise unanswerable from the output.
 */
function renderVersion(json) {
  const source = path.resolve(import.meta.dirname, '..');
  if (json) {
    return JSON.stringify({ name: 'refactor-me', version: VERSION, node: process.version, platform: process.platform, source }, null, 2) + '\n';
  }
  return [
    `refactor-me ${VERSION}`,
    `  node    ${process.version} (${process.platform})`,
    `  source  ${source}`,
    '',
  ].join('\n');
}

async function main() {
  const args = parseArgs(process.argv.slice(2));
  if (args.command === 'help') { process.stdout.write(HELP); return EXIT.OK; }
  // Before resolveRepo: asking a tool its version must work outside a git
  // repository, and resolveRepo exits 2 there.
  if (args.version || args.command === 'version') {
    process.stdout.write(renderVersion(args.json));
    return EXIT.OK;
  }

  const repoRoot = resolveRepo();
  const cfg = loadConfig(repoRoot);
  const providers = providerOrder(cfg, args);
  // Before the banner, like the unknown-command check below: a banner followed
  // by a rejection reads as though the run started and then broke.
  const { targets, errors } = normalizeTargets(repoRoot, args.targets);
  if (errors.length) { for (const e of errors) console.error(e); return EXIT.ABORTED; }

  if (args.command === 'report') return cmdReport(repoRoot, args);
  if (args.command === 'clean') return cmdClean(repoRoot);

  // Validate before announcing: a banner followed by a rejection reads as though
  // the run started and then broke.
  if (args.command !== 'run' && args.command !== 'doctor') {
    console.error(`unknown command: ${args.command}`);
    console.error('run `refactor-me help` for usage');
    return EXIT.ABORTED;
  }

  const log = new Logger({ color: process.stderr.isTTY === true });
  log.summary(`refactor-me  repo=${repoRoot}  providers=${providers.join('+')}${targets.length ? `  target=${targets.join(',')}` : ''}`);

  if (args.command === 'doctor') {
    const runDir = S.runDirFor(repoRoot, `doctor-${S.runId()}`);
    log.attach(runDir);
    const rep = await runDoctor({ repoRoot, cfg, runDir, providers, log, live: args.live, targets });
    log.summary(renderDoctor(rep));
    log.close();
    return rep.ok ? EXIT.OK : EXIT.ABORTED;
  }
  const ctx = await runLoop({ repoRoot, cfg, log, providers, live: args.live, forceQuotaAt: args.forceQuotaAt, targets, language: args.language });
  const report = buildReport(ctx, args.language);
  if (args.json) log.json(report);
  else log.summary('\n' + renderSummary(report, ctx.runDir, args.language));
  fs.writeFileSync(path.join(repoRoot, '.refactor', 'last-run.json'),
    JSON.stringify({ runId: ctx.state.runId, toolVersion: report.toolVersion, runDir: ctx.runDir, status: report.status, branch: report.branch, finishedAt: new Date().toISOString() }, null, 2));
  log.close();

  return report.status === 'HALTED_UNSAFE' ? EXIT.HALTED_UNSAFE
    : report.status === 'ABORTED' ? EXIT.ABORTED
      : EXIT.OK;
}

function cmdReport(repoRoot, args) {
  const last = S.readJson(path.join(repoRoot, '.refactor', 'last-run.json'));
  if (!last) { console.error('no run recorded yet'); return EXIT.ABORTED; }
  if (args.json) { process.stdout.write(fs.readFileSync(path.join(last.runDir, 'report.json'), 'utf8')); return EXIT.OK; }
  const report = JSON.parse(fs.readFileSync(path.join(last.runDir, 'report.json'), 'utf8'));
  process.stdout.write(renderMarkdown(report, args.language));
  return EXIT.OK;
}

/** Only worktrees whose run finished cleanly. A halted run keeps its evidence. */
function cmdClean(repoRoot) {
  const runsDir = path.join(repoRoot, '.refactor', 'runs');
  let ids = [];
  try { ids = fs.readdirSync(runsDir); } catch { console.log('nothing to clean'); return EXIT.OK; }
  let removed = 0;
  for (const id of ids) {
    const st = S.readJson(path.join(runsDir, id, 'state.json'));
    if (!st?.worktree) continue;
    const status = st.terminal?.status;
    if (status === 'HALTED_UNSAFE' || status === 'DONE_PARTIAL' || !status) {
      console.log(`keep  ${id}  (${status ?? 'unfinished'} — worktree preserved as evidence)`);
      continue;
    }
    if (fs.existsSync(st.worktree)) { G.worktreeRemove(repoRoot, st.worktree); removed++; console.log(`removed  ${st.worktree}`); }
  }
  G.gitTry(repoRoot, ['worktree', 'prune']);
  console.log(`${removed} worktree(s) removed`);
  return EXIT.OK;
}

main().then((c) => process.exit(c)).catch((e) => {
  console.error(`refactor-me: ${e.message}`);
  process.exit(EXIT.ABORTED);
});
