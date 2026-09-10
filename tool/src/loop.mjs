// loop.mjs — the state machine.
//
// Nine working states, four terminals. Every non-terminal state has exactly
// three kinds of outgoing edge: advance, abandon-candidate (always preceded by
// rollback), and halt. That uniformity is what keeps this file small.
//
// HALTED_UNSAFE is the only state that requires a human. Its predecessor design
// had six approval states plus WRITE_OUTCOME_UNKNOWN and MANUAL_RECOVERY_REQUIRED;
// all of them existed because a writer wrote into a clone whose outcome was
// expensive to determine. In a worktree the outcome is always one
// `git status --porcelain` away and always one `reset --hard` from undone —
// which is also precisely what makes mid-write provider failover safe.

import { renderHandoffHeader } from './report.mjs';
import fs from 'node:fs';
import path from 'node:path';
import { spawnSync } from 'node:child_process';
import * as G from './git.mjs';
import * as V from './validate.mjs';
import * as P from './provider.mjs';
import * as S from './state.mjs';
import { collectFacts, runChecks, rollbackCandidate, GATE_CHECK_COUNT } from './gate.mjs';
import * as Prompts from './prompts.mjs';
import * as Scope from './scope.mjs';
import {
  SCHEMAS, validate,
  enforceExecutionVerdict, enforcePreflightVerdict, enforceReviewVerdict, enforceCharacterizationVerdict,
} from './schemas.mjs';
import { runDoctor, renderDoctor } from './doctor.mjs';

/** Move both the log and the persisted state; they must never disagree. */
function phase(ctx, name) {
  ctx.log.setState(name);
  ctx.state.state = name;
  ctx.store.save();
}

const RISK_ORDER = { L0_LOW: 0, L1_MODERATE: 1, L2_HIGH: 2, L3_CRITICAL: 3, UNKNOWN: 4 };
const TRANSIENT_BACKOFF_MS = [5_000, 20_000];

export const DEFAULT_POLICY = {
  allowed_risks: ['L0_LOW', 'L1_MODERATE', 'L2_HIGH'],
  auto_characterization: true,
  cross_provider_review: true,
  max_cycles: 25,
  max_commits: 20,
  max_consecutive_failures: 3,
  max_attempts_per_fingerprint: 2,
  empty_audits_to_stop: 2,
  max_files_per_candidate: 8,
  max_changed_lines: 600,
  max_audit_candidates: 8,
  max_wall_clock_min: 180,
  cooldown_minutes: 20,
  extra_forbidden_globs: [],
};

class Halt extends Error {
  constructor(status, reason) { super(reason); this.status = status; this.reason = reason; }
}

export async function runLoop(opts) {
  const ctx = await init(opts);
  try {
    await baseline(ctx);
    await cycles(ctx);
    finish(ctx, ctx.state.counters.commits > 0 ? 'DONE' : 'NO_CHANGES', 'no eligible candidates remain');
  } catch (e) {
    if (e instanceof Halt) finish(ctx, e.status, e.reason);
    else { ctx.log.fail(`unexpected error: ${e.message}`); finish(ctx, 'ABORTED', `internal error: ${e.message}`); throw e; }
  } finally {
    teardown(ctx);
  }
  return ctx;
}

// ---------------------------------------------------------------- INIT

async function init({ repoRoot, cfg, log, providers, live = true, forceQuotaAt = null, targets = [], language = 'en' }) {
  log.setState('INIT');
  const policy = { ...DEFAULT_POLICY, ...(cfg.policy ?? {}) };
  const id = S.runId();
  const runDir = S.runDirFor(repoRoot, id);
  log.attach(runDir);

  const lock = new S.Lock(path.join(repoRoot, '.refactor', 'lock.json'));
  const got = lock.acquire();
  if (!got.ok) throw new Halt('ABORTED', `another run holds the lock (pid ${got.holder?.pid}, started ${got.holder?.startedAt})`);

  const doctor = await runDoctor({ repoRoot, cfg, runDir, providers, log, live, targets });
  log.summary(renderDoctor(doctor));
  if (!doctor.ok) { lock.release(); throw new Halt('ABORTED', 'doctor found a blocking problem'); }

  const baseOid = G.headOid(repoRoot);
  const baseBranch = G.currentBranch(repoRoot);
  const sourceFp = G.sourceFingerprint(repoRoot);
  const branchName = `${cfg.workspace?.branch_prefix ?? 'refactor/auto-'}${id}`;
  const wt = G.worktreePath(repoRoot, id, cfg.workspace?.worktree_parent);

  const ring = S.newProviderRing(P.PROVIDERS);
  for (const p of P.PROVIDERS) if (!doctor.healthy.includes(p)) ring[p].status = 'DISABLED';

  const store = new S.Store(runDir);
  const state = store.init(S.newState({
    id, repoRoot, worktree: wt, baseOid, baseBranch, branchName, providers: ring, targets,
    providerOrder: providers.filter((p) => doctor.healthy.includes(p)),
  }));

  // The doctor probes ran before this state existed, so their spend is carried
  // across from doctor.json rather than lost. It is one small turn per provider,
  // but it happens on every run and the report claims to be a total.
  for (const u of doctor.usage ?? []) {
    P.noteUsage(state, { ...u, phase: 'doctor' });
  }
  store.save();

  log.start(`HEAD ${baseOid.slice(0, 7)} (${baseBranch ?? 'detached'}) · clean · lock ${process.pid}`);
  G.worktreeAdd(repoRoot, wt, baseOid);
  log.pass(`worktree ${wt}`);
  hydrate(repoRoot, wt, log);

  return { repoRoot, wt, runDir, cfg, language, policy, log, store, state, lock, sourceFp, doctor, targets, startedAt: Date.now(), forceQuotaAt };
}

/**
 * Copy the gitignored build inputs a worktree needs. On APFS `cp -c` is a
 * clonefile: metadata-time, no data copy. This is the entire replacement for the
 * predecessor's content-addressed dependency snapshot subsystem — which is safe
 * precisely because every manifest and lockfile is hard-forbidden by the gate,
 * so dependencies cannot change and therefore need no snapshotting.
 */
function hydrate(repoRoot, wt, log) {
  const wanted = ['node_modules', '.env.local', '.env', 'vendor', '.next/cache'];
  const copied = [];
  const walk = (rel, depth) => {
    if (depth > 3) return;
    const abs = path.join(repoRoot, rel);
    let entries = [];
    try { entries = fs.readdirSync(abs, { withFileTypes: true }); } catch { return; }
    for (const e of entries) {
      const childRel = rel ? path.join(rel, e.name) : e.name;
      if (wanted.includes(e.name)) {
        const dst = path.join(wt, childRel);
        if (fs.existsSync(dst)) continue;
        const src = path.join(repoRoot, childRel);
        fs.mkdirSync(path.dirname(dst), { recursive: true });
        // -c is an APFS clonefile: metadata-time, no data copied. Fall back to a
        // plain recursive copy on filesystems that do not support it.
        const cloned = spawnSync('cp', ['-c', '-R', src, dst], { stdio: 'ignore' }).status === 0;
        if (!cloned && spawnSync('cp', ['-R', src, dst], { stdio: 'ignore' }).status !== 0) continue;
        copied.push(`${childRel}${cloned ? '' : ' (plain copy)'}`);
        continue;
      }
      if (!e.isDirectory() || e.name.startsWith('.') || e.name === 'node_modules') continue;
      walk(childRel, depth + 1);
    }
  };
  walk('', 0);
  if (copied.length) log.pass(`hydrated ${copied.join(', ')}`);
  else log.info('no gitignored build inputs to hydrate');
}

// ---------------------------------------------------------------- BASELINE

/** The argv[0] a baseline result came from, for a message a person can act on. */
const cmdOf = (disc, id) => disc.commands.find((c) => c.id === id)?.argv[0] ?? id;

async function baseline(ctx) {
  phase(ctx, 'BASELINE');
  const lockedFile = path.join(ctx.repoRoot, '.refactor', 'commands.json');
  const disc = V.discoverCommands(ctx.wt, { lockedFile });
  ctx.commands = disc.commands;
  ctx.areas = disc.areas;
  ctx.skippedCommands = disc.skipped;
  ctx.hints = disc.hints;

  ctx.log.start(`areas: ${disc.areas.join(', ')}${disc.locked ? ' (commands.json locked)' : ''}`);
  for (const s of disc.skipped) ctx.log.warn(`excluded from the loop: ${s.area} ${s.name} — ${s.reason}`);
  if (disc.commands.length === 0) throw new Halt('ABORTED', 'no deterministic validation command could be discovered');

  const b = await V.runBaseline(disc.commands, ctx.wt, ctx.log);
  ctx.baseline = b;

  // Two ways a command produces no evidence, dropped by the same rule for the
  // same reason: an empty signature compares equal to itself forever, so either
  // one left in the loop would pose as a gate while validating nothing. They are
  // reported apart because the remedies differ — install the tool, versus find
  // out why the tool cannot say anything.
  const noEvidence = [
    ...b.unrunnable.map((r) => [r.id, `${cmdOf(disc, r.id)} could not be executed (${r.spawnError})`]),
    ...(b.opaque ?? []).map((r) => [r.id, `exited ${r.exitCode} with no recognisable error output, so it cannot serve as a differential check`]),
  ];
  if (noEvidence.length) {
    const why = new Map(noEvidence);
    ctx.skippedCommands = [
      ...ctx.skippedCommands,
      ...disc.commands.filter((c) => why.has(c.id)).map((c) => ({ ...c, tier: 'SKIPPED', reason: why.get(c.id) })),
    ];
    ctx.commands = disc.commands.filter((c) => !why.has(c.id));
  }

  S.writeJsonAtomic(path.join(ctx.runDir, 'baseline.json'), {
    areas: disc.areas, commands: ctx.commands, skipped: ctx.skippedCommands, hints: disc.hints,
    results: b.results.map(({ perFile, ...r }) => r),
  });
  if (!b.usable) {
    const missing = [
      ...b.unrunnable.map((r) => `${r.argv[0]} (${r.spawnError})`),
      ...(b.opaque ?? []).map((r) => `${r.argv[0]} (exit ${r.exitCode}, no readable output)`),
    ].join(', ');
    throw new Halt('ABORTED', b.red === 0
      ? `no validation command could be executed: ${missing}`
      : 'every validation command failed at baseline; with no trustworthy signal there is no behavior-preservation evidence to be had'
        + (missing ? ` · also not executable: ${missing}` : ''));
  }
  ctx.log.pass(`${b.describe} → proceeding`);
}

// ---------------------------------------------------------------- cycles

async function cycles(ctx) {
  const { state, policy, log } = ctx;
  while (true) {
    budgetCheck(ctx);
    state.cycle += 1;
    state.counters.cycles += 1;
    log.setCycle(state.cycle);
    ctx.store.save();

    const candidates = await audit(ctx);
    if (candidates.length === 0) {
      state.counters.emptyAudits += 1;
      log.info(`no eligible candidate (empty audit ${state.counters.emptyAudits}/${policy.empty_audits_to_stop})`);
      if (state.counters.emptyAudits >= policy.empty_audits_to_stop) return;
      continue;
    }
    state.counters.emptyAudits = 0;
    await runCandidate(ctx, candidates[0]);
  }
}

function budgetCheck(ctx) {
  const { state, policy } = ctx;
  const mins = (Date.now() - ctx.startedAt) / 60000;
  const over =
    state.counters.cycles >= policy.max_cycles ? `cycle budget (${policy.max_cycles})`
      : state.counters.commits >= policy.max_commits ? `commit budget (${policy.max_commits})`
        : state.counters.consecutiveFailures >= policy.max_consecutive_failures ? `${policy.max_consecutive_failures} consecutive failures`
          : mins >= policy.max_wall_clock_min ? `wall clock (${policy.max_wall_clock_min}m)`
            : P.readyProviders(state).length === 0 ? 'all providers exhausted'
              : null;
  if (over) throw new Halt('DONE_PARTIAL', `stopped on ${over}`);
}

// ---------------------------------------------------------------- provider call

/**
 * One phase, with the provider ring applied.
 *  - transient: retry the same provider twice, then move on
 *  - hard quota / auth: switch providers and retry the SAME phase
 *  - schema (after its own repair): offer the phase to the other provider once
 * A quota hit never counts as a candidate failure; it is a resource event, not
 * evidence about the candidate.
 */
async function callPhase(ctx, { phase, schema, build, prefer, cycleDir }) {
  const { state, policy, log } = ctx;
  const tried = new Set();
  let resourceOnly = true;   // stays true if every provider fell over on quota/auth
  for (let hop = 0; hop < P.PROVIDERS.length; hop++) {
    const provider = P.pickProvider(state, { prefer: hop === 0 ? prefer : undefined, exclude: [...tried][0] });
    if (!provider || tried.has(provider)) break;
    tried.add(provider);

    for (let attempt = 0; attempt <= TRANSIENT_BACKOFF_MS.length; attempt++) {
      P.noteCall(state, provider);
      ctx.store.save();

      // Rehearsal injection. It must produce the SAME result object a real quota
      // failure produces and fall through the same handling below — an earlier
      // version broke out of the loop here, which skipped the handoff brief and
      // therefore rehearsed everything except the part worth rehearsing.
      const injected = ctx.forceQuotaAt === phase && !ctx._forced;
      if (injected) ctx._forced = true;

      const res = injected
        ? { ok: false, failure: 'QUOTA', hard: true, fatal: false, detail: 'injected for rehearsal', provider }
        : await P.callWithRepair(provider, {
          phase, mode: P.modeFor(phase), cwd: ctx.wt,
          body: build(provider), schema,
          runDir: cycleDir ?? ctx.runDir, attempt: 'primary',
          // A re-audit reads a repository it has already surveyed, so it is the
          // one phase whose effort can safely fall after the first cycle.
          effort: phase === 'audit' && state.cycle > 1 ? 'medium' : undefined,
        }, ctx.cfg, log);

      // Recorded before the success check, so a call that timed out or hit a
      // quota still shows up in what the run cost. A rehearsal injection never
      // ran a process, so it has nothing to record.
      if (!injected) {
        P.noteUsage(state, {
          provider, phase, usage: res.usage,
          durationMs: res.durationMs, processes: res.processes, ok: res.ok,
        });
        ctx.store.save();
        log.info(P.usageLine(phase, provider, res), {
          kind: 'usage', provider, phase, ok: res.ok, failure: res.failure ?? null,
          ms: res.durationMs, processes: res.processes,
          inputTokens: res.usage?.inputTokens ?? 0,
          outputTokens: res.usage?.outputTokens ?? 0,
          costUsd: res.usage?.costUsd ?? null,
        });
      }

      if (res.ok) return { res, provider };

      if (res.fatal) throw new Halt('ABORTED', `${provider}: ${res.detail}`);

      if (res.failure === 'QUOTA' || res.failure === 'AUTH') {
        log.warn(`${provider} ${res.failure}: ${res.detail ?? ''}`);
        P.noteFailure(state, provider, res, policy.cooldown_minutes);
        ctx.store.save();
        await writeHandoff(ctx, provider, res.failure);
        break;                                          // hop to the other provider
      }
      if (res.failure === 'TIMEOUT' || res.failure === 'PROCESS') {
        resourceOnly = false;
        if (attempt < TRANSIENT_BACKOFF_MS.length) {
          log.retry(`${provider} ${res.failure} (${res.detail ?? ''}) — retry ${attempt + 1}`);
          await sleep(TRANSIENT_BACKOFF_MS[attempt]);
          continue;
        }
        break;
      }
      // SCHEMA, after its own bounded repair: a different model is the best next move.
      resourceOnly = false;
      log.warn(`${provider} SCHEMA after repair: ${res.detail ?? ''}`);
      break;
    }
  }
  // RESOURCE means nothing was learned about the candidate — only that we ran
  // out of provider. The frozen packet stays eligible so the other provider (or
  // a resumed run after cooldown) can replay it; marking it skipped here would
  // throw away the one artifact that makes the handoff possible.
  return { res: null, provider: null, reason: resourceOnly ? 'RESOURCE' : 'FAILED' };
}

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

/**
 * When a provider drops out mid-run, brief the HUMAN — not the next provider.
 * The next provider needs the frozen task packet, and already has it; sharpen-brief is
 * explicitly for someone who walked away and came back, so this is the one
 * artifact the loop renders for a person rather than for a machine. Written once
 * per run, on whichever provider is still alive.
 */
async function writeHandoff(ctx, deadProvider, failureClass) {
  if (ctx._handoffWritten) return;
  const live = P.pickProvider(ctx.state, { exclude: deadProvider });
  if (!live) return;                       // nobody left to write it
  ctx._handoffWritten = true;

  const journal = path.join(ctx.runDir, 'events.ndjson');
  let tail = '';
  try { tail = fs.readFileSync(journal, 'utf8').split('\n').slice(-60).join('\n'); } catch { /* none yet */ }

  const res = await P.callProvider(live, {
    phase: 'handoff', mode: 'read', cwd: ctx.wt,
    body: Prompts.handoffPrompt({
      provider: live, nonce: Prompts.newNonce(),
      deadProvider, liveProvider: live, failureClass,
      stateJson: JSON.stringify({ ...ctx.state, treeHashes: undefined }, null, 2),
      journalTail: tail,
      gitLog: G.logOneline(ctx.repoRoot, `${ctx.state.baseOid}..${ctx.state.publishedOid ?? ctx.state.baseOid}`, 20),
    }),
    schema: null, runDir: ctx.runDir, attempt: 'primary', effort: 'low',
  }, ctx.cfg, null);

  // The handoff brief is a real provider call and used to be invisible in every
  // total. It is small (effort: low) but it happens on exactly the runs a reader
  // is trying to understand.
  P.noteUsage(ctx.state, {
    provider: live, phase: 'handoff', usage: res.usage,
    durationMs: res.durationMs, processes: res.processes, ok: res.ok,
  });
  ctx.store.save();

  if (res.ok && res.text.trim()) {
    // Written mid-run, at the moment a provider dropped out. The run then keeps
    // going, so this brief goes stale by design — say so in the file rather than
    // hoping the reader infers it, and name the artifact that is authoritative.
    const header = renderHandoffHeader(deadProvider, live, failureClass, ctx.language);
    fs.writeFileSync(path.join(ctx.runDir, 'handoff.md'), header + res.text);
    ctx.log.info(`handoff brief written for you: ${path.join(ctx.runDir, 'handoff.md')}`);
  }
}

// ---------------------------------------------------------------- AUDIT

async function audit(ctx) {
  const { state, policy, log } = ctx;
  phase(ctx, 'AUDIT');
  const incremental = state.cycle > 1;
  const nonce = Prompts.newNonce();
  const repoFacts = Prompts.collectRepoFacts(ctx.wt, {
    areas: ctx.areas, commands: ctx.commands, skipped: ctx.skippedCommands, targets: ctx.targets,
  });

  log.start(`${incremental ? 'incremental re-audit' : 'full audit'} · ${state.counters.commits} committed, ${state.seen.skipped.length} abandoned`);
  const { res, provider, reason } = await callPhase(ctx, {
    phase: 'audit', schema: SCHEMAS.audit,
    build: (p) => Prompts.auditPrompt({
      provider: p, nonce, incremental, repoFacts, cycle: state.cycle, targets: ctx.targets,
      seen: state.seen, violated: violatedPaths(state),
    }),
  });
  if (!res) {
    if (reason !== 'RESOURCE') state.counters.consecutiveFailures += 1;
    ctx.store.save();
    return [];
  }

  const all = (res.data.candidates ?? []).slice(0, policy.max_audit_candidates);
  S.writeJsonAtomic(path.join(ctx.runDir, 'audits', `${String(state.cycle).padStart(2, '0')}.json`), { provider, ...res.data });
  const byCat = {};
  for (const c of all) byCat[c.category] = (byCat[c.category] ?? 0) + 1;
  log.pass(`${all.length} candidate(s): ${Object.entries(byCat).map(([k, v]) => `${k} ${v}`).join(', ') || 'none'}`);

  return rank(ctx, all);
}

function violatedPaths(state) {
  return [...new Set(state.seen.skipped.flatMap((s) => s.detail?.paths ?? []))];
}

/**
 * Controller-side ranking. The model proposes; the loop decides, using a
 * fingerprint computed from (kind, path-set, symbol) rather than from prose, so
 * a re-proposal with new wording still collides with the old one.
 * Exported for tests: this is where a bad filter silently starves the loop.
 */
export function rank(ctx, candidates) {
  const { state, policy, log } = ctx;
  const eligible = [];
  for (const c of candidates) {
    c.fp = S.fingerprint({ kind: c.category, paths: c.related_files, symbol: c.primary_symbol });
    // Scope first, because "outside the folder you asked about" is the more
    // useful thing to tell the operator than whatever secondary objection this
    // candidate would also have failed on. A candidate needs only ONE foot
    // inside a target: the callers and tests it must also edit are legitimately
    // outside, and rejecting those would leave the repository uncompilable.
    if (!Scope.touchesTarget(c.related_files, ctx.targets)) {
      note(ctx, c, 'OUT_OF_TARGET', `no related file under ${(ctx.targets ?? []).join(', ')}`); continue;
    }
    // UNKNOWN is not "outside the allowed range"; it is the absence of a
    // judgement. Reported separately so the operator reads it as evidence still
    // owed rather than as a candidate that was too dangerous.
    if (c.risk_level === 'UNKNOWN') {
      note(ctx, c, 'RISK_UNKNOWN', 'sharpen-assess could not support a risk level from the available evidence'); continue;
    }
    if (!policy.allowed_risks.includes(c.risk_level)) {
      note(ctx, c, 'RISK_EXCLUDED', `${c.risk_level} is outside allowed_risks`); continue;
    }
    if (c.readiness === 'REJECT') { note(ctx, c, 'MODEL_REJECTED', c.problem); continue; }
    if (S.isSeen(state, c.fp)) continue;
    if (S.attemptsOf(state, c.fp) >= policy.max_attempts_per_fingerprint) {
      note(ctx, c, 'ATTEMPTS_EXHAUSTED', `${policy.max_attempts_per_fingerprint} attempts`); continue;
    }
    if ((c.estimated_file_count ?? 1) > policy.max_files_per_candidate) {
      note(ctx, c, 'TOO_LARGE', `${c.estimated_file_count} files`); continue;
    }
    eligible.push(c);
  }
  eligible.sort((a, b) =>
    (RISK_ORDER[a.risk_level] - RISK_ORDER[b.risk_level]) ||
    ((a.readiness === 'READY' ? 0 : 1) - (b.readiness === 'READY' ? 0 : 1)) ||
    ((a.estimated_file_count ?? 9) - (b.estimated_file_count ?? 9)) ||
    a.candidate_id.localeCompare(b.candidate_id));
  if (eligible.length) {
    const w = eligible[0];
    log.info(`${candidates.length - eligible.length} filtered · picked ${w.candidate_id} fp=${w.fp}`);
  }
  ctx.store.save();
  return eligible;
}

function note(ctx, c, reason, detail) {
  S.markSkipped(ctx.state, c.fp, reason, { detail, paths: c.related_files });
  ctx.log.info(`skip ${c.candidate_id} (${reason}${detail ? ': ' + String(detail).slice(0, 80) : ''})`);
}

// ---------------------------------------------------------------- candidate

async function runCandidate(ctx, cand) {
  const { state, log } = ctx;
  const cycleDir = S.cycleDirFor(ctx.runDir, state.cycle, cand.category, cand.fp);
  ctx.cycleDir = cycleDir;
  S.noteAttempt(state, cand.fp);

  const packet = await select(ctx, cand, cycleDir);
  if (!packet) return;

  state.activePacket = packet;
  ctx.store.save();

  const preOid = G.headOid(ctx.wt);

  if (packet.characterization_needed && ctx.policy.auto_characterization) {
    const ok = await characterize(ctx, packet, cycleDir, preOid);
    if (!ok) return;
  }

  if (!await preflight(ctx, packet, cycleDir)) return;

  const writeOid = G.headOid(ctx.wt);
  const exec = await execute(ctx, packet, cycleDir, writeOid);
  if (!exec) return;

  const gated = await gate(ctx, packet, cycleDir, writeOid);
  if (!gated) return;

  const reviewed = await review(ctx, packet, cycleDir, exec);
  if (!reviewed) return;

  commit(ctx, packet, cycleDir, cand);
}

// ---------------------------------------------------------------- SELECT

async function select(ctx, cand, cycleDir) {
  const { log, state } = ctx;
  phase(ctx, 'SELECT');
  log.start(`${cand.category} ${cand.risk_level} ${cand.candidate_id} — ${(cand.related_files ?? []).slice(0, 3).join(', ')}`);

  const nonce = Prompts.newNonce();
  const { res, reason } = await callPhase(ctx, {
    phase: 'deep_check', schema: SCHEMAS.deepcheck, cycleDir,
    build: (p) => Prompts.deepCheckPrompt({
      provider: p, nonce, candidate: cand,
      repoFacts: Prompts.collectRepoFacts(ctx.wt, { areas: ctx.areas, commands: ctx.commands, skipped: ctx.skippedCommands, maxFiles: 200, targets: ctx.targets }),
      areaSkills: Prompts.collectAreaSkills(ctx.wt, cand.related_files ?? []),
      targets: ctx.targets,
      commands: ctx.commands.map((c) => `  ${c.area} ${c.tier} ${c.name}: ${c.argv.join(' ')}`).join('\n'),
    }),
  });
  if (!res) return outOfProvider(ctx, reason) ? null : fail(ctx, cand.fp, 'DEEP_CHECK_FAILED', 'no provider produced a task packet');

  const packet = res.data;
  packet.fp = cand.fp;
  packet.original_lines = G.fileLineCount(ctx.wt, packet.allowlist?.[0] ?? '') ?? null;
  S.writeJsonAtomic(path.join(cycleDir, 'packet.json'), packet);

  if (packet.readiness !== 'READY') {
    S.markSkipped(state, cand.fp, 'NOT_READY', { detail: packet.readiness_reason, paths: cand.related_files });
    log.info(`deep-check ${packet.readiness}: ${String(packet.readiness_reason).slice(0, 120)}`);
    ctx.store.save();
    return null;                                   // not a failure: the gate did its job
  }
  if (!packet.allowlist?.length) return fail(ctx, cand.fp, 'EMPTY_ALLOWLIST', 'READY packet with no allowlist');

  log.pass(`packet frozen · ${packet.allowlist.length} path(s)${packet.characterization_needed ? ' · characterization first' : ''}`);
  return packet;
}

// ---------------------------------------------------------------- CHARACTERIZE

async function characterize(ctx, packet, cycleDir, preOid) {
  const { log } = ctx;
  phase(ctx, 'CHARACTERIZE');
  if (!packet.characterization_files?.length) {
    return fail(ctx, packet.fp, 'NO_CHARACTERIZATION_FILES', 'characterization required but no test files named');
  }
  log.start(`test-only slice · ${packet.characterization_files.join(', ')}`);
  const nonce = Prompts.newNonce();
  const { res, reason } = await callPhase(ctx, {
    phase: 'characterization', schema: SCHEMAS.characterization, cycleDir,
    build: (p) => Prompts.characterizationPrompt({
      provider: p, nonce, packet,
      areaSkills: Prompts.collectAreaSkills(ctx.wt, packet.allowlist),
    }),
  });
  if (!res) {
    rollback(ctx, preOid);
    if (outOfProvider(ctx, reason)) return false;
    return fail(ctx, packet.fp, 'CHARACTERIZATION_FAILED', 'no provider completed the test slice');
  }

  const verdict = enforceCharacterizationVerdict(res.data);
  sweep(ctx);
  const changed = G.changedPaths(ctx.wt);
  const allowed = new Set(packet.characterization_files);
  const outside = changed.filter((f) => !allowed.has(f));

  if (verdict.verdict === 'FAIL' || outside.length > 0) {
    // Same gap as EXECUTE had: when the model self-reports FAIL there are no
    // enforced reasons, and printing an empty list cost a cycle of not knowing why.
    const why = outside.length
      ? `touched ${outside.join(', ')} outside the test allowlist${untrackedHint(ctx, outside)}`
      : (verdict.reasons.join('; ') || res.data.notes || 'the implementer reported FAIL without a reason');
    log.fail(`characterization rejected: ${why}`);
    rollback(ctx, preOid);
    return fail(ctx, packet.fp, 'CHARACTERIZATION_REJECTED', why);
  }
  if (changed.length === 0) { log.info('characterization produced no new tests; continuing'); return true; }

  // The new tests must pass against UNMODIFIED production code, or they are not
  // characterizing current behavior — they are describing a wish.
  const ladder = await V.runLadder(ctx.baseline, ctx.commands, ctx.wt, changed, ctx.areas, log);
  if (!ladder.ok) {
    log.fail('characterization tests do not pass against unmodified code');
    rollback(ctx, preOid);
    return fail(ctx, packet.fp, 'CHARACTERIZATION_RED', ladder.failed?.why ?? 'ladder failed');
  }

  G.stageExact(ctx.wt, changed);
  const msgFile = path.join(cycleDir, 'characterization-message.txt');
  fs.writeFileSync(msgFile, `test(${packet.candidate_id}): characterize current behavior before refactor\n\n` +
    `${(res.data.characterized_contracts ?? []).map((c) => `- ${c}`).join('\n')}\n\nRefactor-Fingerprint: ${packet.fp}-char\n`);
  const oid = G.commit(ctx.wt, msgFile);
  publish(ctx, oid);
  ctx.state.treeHashes.push(G.git(ctx.wt, ['rev-parse', 'HEAD^{tree}']).trim());
  // Recorded on the same footing as a refactor commit, because it is real work
  // that landed on the branch. It does NOT count toward the commit budget: the
  // budget bounds how much production code a run may change, and a test-only
  // slice changes none. Omitting it entirely made a run that had published a
  // branch report "nothing was committed".
  ctx.state.commits.push({
    oid, fp: `${packet.fp}-char`, category: 'CHARACTERIZATION',
    paths: changed, subject: `characterize current behavior for ${packet.candidate_id}`,
  });
  log.pass(`${oid.slice(0, 7)}  test(${packet.candidate_id}): characterization committed separately`);
  ctx.store.save();
  return true;
}

// ---------------------------------------------------------------- PREFLIGHT

async function preflight(ctx, packet, cycleDir) {
  const { log } = ctx;
  phase(ctx, 'PREFLIGHT');
  const nonce = Prompts.newNonce();
  const { res, reason } = await callPhase(ctx, {
    phase: 'preflight', schema: SCHEMAS.preflight, cycleDir,
    build: (p) => Prompts.preflightPrompt({
      provider: p, nonce, packet,
      repoFacts: Prompts.collectRepoFacts(ctx.wt, { areas: ctx.areas, commands: ctx.commands, skipped: ctx.skippedCommands, maxFiles: 120, targets: ctx.targets }),
      baselineSummary: ctx.baseline.describe,
    }),
  });
  if (!res) return outOfProvider(ctx, reason) ? false : Boolean(fail(ctx, packet.fp, 'PREFLIGHT_FAILED', 'no provider completed preflight'));

  S.writeJsonAtomic(path.join(cycleDir, 'preflight.json'), res.data);
  const v = enforcePreflightVerdict(res.data);
  if (v.verdict !== 'READY_TO_EXECUTE') {
    log.info(`BLOCKED — ${v.reasons.join('; ')} · objection stands: ${String(res.data.failure_hypothesis).slice(0, 90)}`);
    S.markSkipped(ctx.state, packet.fp, 'PREFLIGHT_BLOCKED', { detail: v.reasons.join('; '), paths: packet.allowlist });
    ctx.store.save();
    return false;                                   // a killed plan is a cheap, correct outcome
  }
  // The verdict has to lead. Writing the hypothesis first read as "here is a
  // problem" — a handoff brief generated from this journal reported the
  // candidate as blocked when preflight had in fact cleared it.
  log.pass(`CLEARED — strongest objection ruled out: ${String(res.data.failure_hypothesis).slice(0, 90)}`);
  return true;
}

// ---------------------------------------------------------------- EXECUTE

async function execute(ctx, packet, cycleDir, preOid) {
  const { log } = ctx;
  phase(ctx, 'EXECUTE');
  const nonce = Prompts.newNonce();
  const { res, provider, reason } = await callPhase(ctx, {
    phase: 'execute', schema: SCHEMAS.execute, cycleDir,
    build: (p) => Prompts.executePrompt({
      provider: p, nonce, packet,
      areaSkills: Prompts.collectAreaSkills(ctx.wt, packet.allowlist),
    }),
  });
  if (!res) {
    // Includes the quota case. A worktree makes this cheap: whatever the writer
    // managed to do is one `reset --hard` from gone, so the packet can simply be
    // replayed on the other provider on the next attempt.
    rollback(ctx, preOid);
    if (outOfProvider(ctx, reason)) return null;
    return fail(ctx, packet.fp, 'EXECUTE_FAILED', 'no provider completed the write');
  }
  S.writeJsonAtomic(path.join(cycleDir, 'execution.json'), { provider, ...res.data });

  const v = enforceExecutionVerdict(res.data, packet);
  if (v.verdict === 'FAIL') {
    // When the model self-reports FAIL there are no enforced reasons to print,
    // and logging an empty list cost a full cycle of not knowing why. Its own
    // words are the only explanation that exists — surface them.
    const why = v.reasons.length > 0
      ? v.reasons.join('; ')
      : (res.data.scope_expansion_reason || res.data.notes || res.data.rationale || 'the implementer reported FAIL without a reason');
    log.fail(`implementer stopped: ${why}`);
    rollback(ctx, preOid);
    return fail(ctx, packet.fp, 'EXECUTE_REJECTED', why);
  }

  // Deletions are declared, not performed: no provider is given a deletion
  // primitive, so the orchestrator applies them here under the allowlist it
  // already froze. Idempotent, because a provider whose sandbox CAN delete may
  // have done it already.
  const removed = applyDeclaredDeletions(ctx, packet, res.data.deleted_files ?? []);
  if (removed === null) {
    rollback(ctx, preOid);
    return fail(ctx, packet.fp, 'OUT_OF_SCOPE', 'declared a deletion outside the allowlist');
  }

  const parts = [`${res.data.changed_files.length} edited`];
  if (removed.length) parts.push(`${removed.length} deleted`);
  log.pass(`${parts.join(', ')} · ${res.data.hunks.length} hunk(s) all accounted for · ${res.toolCalls} tool calls`);
  return { ...res.data, provider };
}

/**
 * Apply the deletions the model declared. Returns the paths actually removed, or
 * null if any declared path lies outside the frozen allowlist — which is a scope
 * violation exactly as if the model had edited the file itself.
 */
export function applyDeclaredDeletions(ctx, packet, declared) {
  if (declared.length === 0) return [];
  const allow = new Set(packet.allowlist ?? []);
  const outside = declared.filter((f) => !allow.has(f));
  if (outside.length > 0) {
    ctx.log.fail(`declared deletion outside the allowlist: ${outside.join(', ')}`);
    return null;
  }
  const removed = [];
  for (const rel of declared) {
    const abs = path.join(ctx.wt, rel);
    if (!fs.existsSync(abs)) continue;         // a sandbox that can delete already did
    fs.rmSync(abs, { force: true });
    removed.push(rel);
    ctx.log.info(`deleted ${rel} (declared by the implementer)`);
  }
  return removed;
}

// ---------------------------------------------------------------- GATE

async function gate(ctx, packet, cycleDir, preOid) {
  const { log, state } = ctx;
  phase(ctx, 'GATE');
  sweep(ctx);
  // collectFacts stages the change set so the numbers include new files.
  const facts = collectFacts({ wt: ctx.wt, repoRoot: ctx.repoRoot, preOid, baseOid: state.baseOid, sourceFp: ctx.sourceFp, packet });
  if (facts.changed.length > 0) facts.prospectiveTree = G.writeTree(ctx.wt);

  const r = runChecks(facts, packet, ctx.policy, state, ctx.targets);
  r.checks.forEach((c, i) => {
    // Denominator is the full sequence, not how many happened to run: "8/8" on a
    // failure reads as "all eight passed".
    const label = `${i + 1}/${GATE_CHECK_COUNT} ${c.id}`;
    if (c.ok) log.pass(`${label}: ${c.detail}`);
    else log.fail(`${label}: ${c.detail}`);
  });
  S.writeJsonAtomic(path.join(cycleDir, 'gate.json'), { verdict: r.verdict, checks: r.checks, violation: r.violation });

  if (r.verdict === 'HALT') {
    throw new Halt('HALTED_UNSAFE', `${r.violation.code}: ${r.violation.detail}`);
  }
  if (r.verdict === 'NO_OP') {
    rollback(ctx, preOid);
    S.markSkipped(state, packet.fp, 'NO_OP', { detail: 'the implementer changed nothing', paths: packet.allowlist });
    ctx.store.save();
    return false;
  }
  if (r.verdict === 'VIOLATION') {
    savePatch(ctx, cycleDir);
    rollback(ctx, preOid);
    state.counters.violations += 1;
    return fail(ctx, packet.fp, r.violation.code, r.violation.detail, r.violation.paths);
  }

  log.info(`ladder scoped to ${V.areasForPaths(ctx.areas, facts.changed).join(', ')}`);
  const ladder = await V.runLadder(ctx.baseline, ctx.commands, ctx.wt, facts.changed, ctx.areas, log);
  S.writeJsonAtomic(path.join(cycleDir, 'validation.json'), {
    ok: ladder.ok, scope: ladder.scope,
    checks: ladder.checks.map(({ perFile, delta, ...c }) => ({ ...c, delta: delta ? { added: delta.added, drifted: delta.drifted } : null })),
  });
  if (!ladder.ok) {
    savePatch(ctx, cycleDir);
    rollback(ctx, preOid);
    return fail(ctx, packet.fp, ladder.failed?.failureKind ?? 'VALIDATION_FAIL', ladder.failed?.why ?? 'validation failed');
  }
  ctx.gateFacts = facts;
  return true;
}

/**
 * Where the build tools we are about to run write. Derived from the discovered
 * commands rather than listed as constants, so an area whose build we never call
 * keeps its `build/` directory untouched.
 */
function buildOutputDirs(commands) {
  const out = new Set();
  for (const c of commands ?? []) {
    const base = c.area === '.' ? '' : `${c.area}/`;
    if (c.family === 'gradle') { out.add(`${base}build`); out.add(`${base}.gradle`); }
    if (c.family === 'maven') out.add(`${base}target`);
  }
  return [...out];
}

/**
 * Clear compiled artifacts a validation command left behind, so they are not
 * mistaken for the implementer's edits.
 */
function sweep(ctx) {
  const swept = G.sweepBuildArtifacts(ctx.wt, { outputDirs: buildOutputDirs(ctx.commands) });
  if (swept.length === 0) return;
  const shown = swept.slice(0, 3).map((s) => `${s.path} (${Math.round(s.bytes / 1024)}KB)`).join(', ');
  ctx.log.info(`swept ${swept.length} build artifact(s): ${shown}${swept.length > 3 ? ' …' : ''} — add these to .gitignore to keep them out of the change set`);
}

/**
 * A path outside the allowlist that git does not track is usually a byproduct
 * rather than scope creep, and saying so turns a confusing rejection into an
 * actionable one.
 */
function untrackedHint(ctx, paths) {
  const untracked = new Set(G.git(ctx.wt, ['ls-files', '--others', '--exclude-standard']).split('\n').filter(Boolean));
  const news = paths.filter((p) => untracked.has(p));
  return news.length
    ? ` — ${news.join(', ')} ${news.length === 1 ? 'is' : 'are'} untracked, so this is likely a build byproduct rather than scope creep; gitignore it and re-run`
    : '';
}

function savePatch(ctx, cycleDir) {
  try { fs.writeFileSync(path.join(cycleDir, 'rejected.patch'), G.diffPatch(ctx.wt)); } catch { /* best effort */ }
}

// ---------------------------------------------------------------- REVIEW

async function review(ctx, packet, cycleDir, exec) {
  const { log, state, policy } = ctx;
  phase(ctx, 'REVIEW');
  // The ladder just ran, and it re-creates whatever the gate's sweep removed.
  sweep(ctx);
  const diff = G.diffForReview(ctx.wt);
  fs.writeFileSync(path.join(cycleDir, 'accepted.patch'), diff);

  // The reviewer is deliberately the provider that did NOT write, when one is
  // available — genuine independence at no extra cost. It is NOT given the
  // implementer's verdict or hunk classifications; a "fresh" session handed
  // those anchors on them and rubber-stamps.
  const reviewer = policy.cross_provider_review ? P.pickReviewer(state, exec.provider) : exec.provider;
  const cross = reviewer && reviewer !== exec.provider;
  log.start(`${reviewer ?? 'none'}${cross ? ' (cross-provider)' : ' (same provider, fresh session)'} · ${Math.round(diff.length / 1024)}KB diff`);

  const nonce = Prompts.newNonce();
  const { res, reason } = await callPhase(ctx, {
    phase: 'review', schema: SCHEMAS.review, cycleDir, prefer: reviewer,
    build: (p) => Prompts.reviewPrompt({ provider: p, nonce, packet, diff, rationale: exec.rationale }),
  });
  if (!res) {
    rollback(ctx, ctx.gateFacts.preOid);
    if (outOfProvider(ctx, reason)) return false;
    return fail(ctx, packet.fp, 'REVIEW_FAILED', 'no provider completed the review');
  }
  S.writeJsonAtomic(path.join(cycleDir, 'review.json'), res.data);

  const v = enforceReviewVerdict(res.data);
  const blockers = (res.data.findings ?? []).filter((f) => f.severity === 'BLOCKER');
  if (v.verdict !== 'PASS') {
    log.fail(`REJECT — ${v.reasons.join('; ')}${blockers[0] ? `: ${blockers[0].claim}` : ''}`);
    savePatch(ctx, cycleDir);
    rollback(ctx, ctx.gateFacts.preOid);
    return fail(ctx, packet.fp, 'REVIEW_REJECT', v.reasons.join('; '));
  }
  const notes = (res.data.findings ?? []).length;
  log.pass(`APPROVE — 0 blocking${notes ? `, ${notes} note(s)` : ''}`);
  return true;
}

// ---------------------------------------------------------------- COMMIT

function commit(ctx, packet, cycleDir, cand) {
  const { log, state } = ctx;
  phase(ctx, 'COMMIT');

  // The commit must contain exactly the tree the gate validated and the reviewer
  // read — no more, no less. Re-stage and re-derive it rather than trusting the
  // index to have survived the review phase untouched.
  G.stageExact(ctx.wt, ctx.gateFacts.changed);
  const staged = G.writeTree(ctx.wt);
  if (staged !== ctx.gateFacts.prospectiveTree) {
    throw new Halt('HALTED_UNSAFE',
      `the working tree changed between validation and commit (approved ${ctx.gateFacts.prospectiveTree.slice(0, 7)}, now ${staged.slice(0, 7)})`);
  }

  const msgFile = path.join(cycleDir, 'message.txt');
  fs.writeFileSync(msgFile, commitMessage(packet, cand, ctx));

  const oid = G.commit(ctx.wt, msgFile);
  const tree = G.git(ctx.wt, ['rev-parse', 'HEAD^{tree}']).trim();
  state.treeHashes.push(tree);
  publish(ctx, oid);

  state.counters.commits += 1;
  state.counters.consecutiveFailures = 0;
  S.markDone(state, packet.fp);
  state.commits.push({
    oid, fp: packet.fp, category: packet.category,
    paths: packet.allowlist, subject: packet.title ?? packet.candidate_id,
  });
  state.activePacket = null;
  ctx.store.save();

  log.pass(`${oid.slice(0, 7)}  ${firstLine(fs.readFileSync(msgFile, 'utf8'))}`);
  log.pass(`published ${state.branchName} → ${oid.slice(0, 7)}`);
}

/**
 * Titles arrive already phrased as an action ("Remove the unreachable legacy
 * parser"), so prefixing a category verb produced "remove Remove the...". Use
 * the model's title, and only supply a verb when it does not open with one.
 */
function commitSubject(packet) {
  const scope = (packet.allowlist?.[0] ?? '').split('/')[0] || 'core';
  const raw = (packet.title ?? packet.candidate_id ?? '').trim().replace(/[.。]$/, '');
  // Titles come back in whatever language the model chose, and a verb can sit at
  // either end of the sentence. Prefixing an English verb onto a Korean title
  // produced "remove 참조되지 않는 ... 삭제" — remove ... delete. Only supply a
  // verb for a Latin-script title that does not already open with one; anything
  // else is used verbatim, because we cannot parse its grammar.
  const latinOnly = /^[\x20-\x7E]*$/.test(raw);
  const opensWithVerb = /^(remove|delete|drop|consolidate|deduplicate|unify|split|extract|inline|merge|simplify|clean|prune)\b/i.test(raw);
  const verb = { DEAD_CODE: 'remove', COMPATIBILITY_REMOVAL: 'remove', DEDUPLICATION: 'consolidate', LARGE_COMPONENT_SPLIT: 'split' }[packet.category] ?? 'refactor';
  const body = (!latinOnly || opensWithVerb)
    ? raw.charAt(0).toLowerCase() + raw.slice(1)
    : `${verb} ${raw}`;
  return `refactor(${scope}): ${body}`;
}

function commitMessage(packet, cand, ctx) {
  return [
    commitSubject(packet),
    '',
    'Why:',
    `- ${packet.minimal_change ?? cand.minimal_change ?? 'structural cleanup'}`,
    '',
    'Preserved:',
    ...(packet.contracts ?? []).map((c) => `- ${c.statement}`),
    '',
    'Verified:',
    ...ctx.commands.map((c) => `- ${c.area} ${c.tier} ${c.name}: ${c.argv.join(' ')}`),
    `- baseline comparison: ${ctx.baseline.describe}`,
    '',
    `Refactor-Fingerprint: ${packet.fp}`,
    '',
  ].join('\n');
}

export { commitSubject };

/** Compare-and-swap the branch ref in the SOURCE repo, one commit at a time. */
function publish(ctx, oid) {
  const ref = `refs/heads/${ctx.state.branchName}`;
  try {
    G.updateRefCas(ctx.repoRoot, ref, oid, ctx.state.publishedOid);
  } catch (e) {
    throw new Halt('HALTED_UNSAFE', `could not publish ${ref}: ${e.message}`);
  }
  ctx.state.publishedOid = oid;
  ctx.store.save();
}

// ---------------------------------------------------------------- helpers

function rollback(ctx, preOid) {
  const r = rollbackCandidate(ctx.wt, preOid);
  if (!r.clean) throw new Halt('HALTED_UNSAFE', `rollback left the worktree dirty: ${r.residue.split('\n')[0]}`);
  ctx.log.retry(`rolled back to ${preOid.slice(0, 7)} · tree clean`);
}

/**
 * True when we ran out of PROVIDER rather than out of candidate. Nothing is
 * marked skipped and no failure is counted: the fingerprint stays eligible so
 * the other provider — or a later run, once a cooldown expires — can replay the
 * already-frozen packet. Marking it skipped here would discard the one artifact
 * that makes a cross-provider handoff possible.
 */
function outOfProvider(ctx, reason) {
  if (reason !== 'RESOURCE') return false;
  ctx.log.warn('providers exhausted for this phase — the candidate stays eligible and its packet is preserved');
  ctx.state.activePacket = null;
  ctx.store.save();
  return true;
}

function fail(ctx, fp, reason, detail, paths) {
  S.markSkipped(ctx.state, fp, reason, { detail, paths });
  ctx.state.counters.consecutiveFailures += 1;
  ctx.state.activePacket = null;
  ctx.store.save();
  ctx.log.fail(`${reason}: ${String(detail ?? '').slice(0, 140)}`);
  return null;
}

const firstLine = (s) => s.split('\n')[0];

function finish(ctx, status, reason) {
  ctx.log.setCycle(null);
  ctx.state.terminal = { status, reason, at: new Date().toISOString() };
  ctx.store.save();
}

function teardown(ctx) {
  try { ctx.lock?.release(); } catch { /* nothing to do */ }
  const keep = ctx.cfg?.workspace?.keep_worktree ?? true;
  const st = ctx.state?.terminal?.status;
  // A halted or partial run keeps its worktree: it is the evidence.
  if (!keep && (st === 'DONE' || st === 'NO_CHANGES')) {
    G.worktreeRemove(ctx.repoRoot, ctx.wt);
    ctx.log.info('worktree removed');
  }
  ctx.log.endHeartbeat();
}
