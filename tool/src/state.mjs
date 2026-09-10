// state.mjs — the resumable run state, candidate fingerprints, and the lock.
// Durability is one rule: write .tmp, fsync, rename. That is the whole
// persistence layer.

import fs from 'node:fs';
import path from 'node:path';
import os from 'node:os';
import crypto from 'node:crypto';
import { newUsage } from './provider.mjs';
import { VERSION } from './version.mjs';

// Stamped into every state.json and never read back: Store.load() does no
// version check. Kept because an old run directory should at least say which
// shape it was written in. `toolVersion` in newState is the field that answers
// "which build", and it is the one to reach for.
export const STATE_SCHEMA = 1;

// ---------------------------------------------------------------- fingerprint

/**
 * Identity of a candidate is (kind, path-set, symbol) — never the prose.
 * A model that re-proposes the same deletion with new wording still collides.
 */
export function fingerprint({ kind, paths, symbol }) {
  const norm = [...new Set((paths ?? []).map((p) => p.replace(/^\.\//, '').replace(/\/+$/, '')))].sort();
  const raw = `${kind}\0${norm.join(',')}\0${(symbol ?? '').toLowerCase()}`;
  return crypto.createHash('sha1').update(raw).digest('hex').slice(0, 12);
}

export function runId(now = new Date()) {
  const p = (n) => String(n).padStart(2, '0');
  const stamp = `${now.getFullYear()}${p(now.getMonth() + 1)}${p(now.getDate())}-${p(now.getHours())}${p(now.getMinutes())}`;
  return `${stamp}-${crypto.randomBytes(2).toString('hex')}`;
}

// ---------------------------------------------------------------- atomic io

export function writeJsonAtomic(file, obj) {
  fs.mkdirSync(path.dirname(file), { recursive: true });
  const tmp = `${file}.tmp`;
  const fd = fs.openSync(tmp, 'w');
  try {
    fs.writeFileSync(fd, JSON.stringify(obj, null, 2) + '\n');
    fs.fsyncSync(fd);
  } finally {
    fs.closeSync(fd);
  }
  fs.renameSync(tmp, file);
}

export function readJson(file, fallback = null) {
  try { return JSON.parse(fs.readFileSync(file, 'utf8')); } catch { return fallback; }
}

// ---------------------------------------------------------------- layout

/**
 * `.refactor/` holds a `.gitignore` containing `*`, which makes the directory —
 * including that file — invisible to `git status`. That is what lets the run
 * write freely inside the target repository without ever dirtying it, and it is
 * why no ownership manifest is needed: nothing we write can be committed by
 * accident. Self-healing, because a run must never be the thing that dirties
 * the tree it is about to assert is clean.
 */
export function ensureRefactorDir(repoRoot) {
  const dir = path.join(repoRoot, '.refactor');
  fs.mkdirSync(dir, { recursive: true });
  const ignore = path.join(dir, '.gitignore');
  if (!fs.existsSync(ignore) || fs.readFileSync(ignore, 'utf8').trim() !== '*') {
    fs.writeFileSync(ignore, '*\n');
  }
  return dir;
}

export function runDirFor(repoRoot, id) {
  const dir = path.join(ensureRefactorDir(repoRoot), 'runs', id);
  fs.mkdirSync(dir, { recursive: true });
  return dir;
}

export function cycleDirFor(runDir, cycle, category, fp) {
  const dir = path.join(runDir, 'cycles', `${String(cycle).padStart(2, '0')}-${category}-${fp}`);
  fs.mkdirSync(path.join(dir, 'provider'), { recursive: true });
  return dir;
}

// ---------------------------------------------------------------- run state

export function newState({ id, repoRoot, worktree, baseOid, baseBranch, branchName, providers, providerOrder, targets = [] }) {
  return {
    schema: STATE_SCHEMA,
    // Which build produced this run. `schema` above is the state file's format
    // version and is a different fact — hence the longer name, which also makes
    // `grep -r toolVersion .refactor/runs` work.
    toolVersion: VERSION,
    runId: id,
    state: 'INIT',
    cycle: 0,
    repoRoot,
    worktree,
    // Which directories this run was scoped to, [] for the whole repository.
    // Recorded because report.md can be rebuilt long after the fact, and "did
    // this run even look at that folder" is otherwise unanswerable.
    targets,
    baseOid,
    baseBranch,
    branchName,
    publishedOid: null,
    startedAt: new Date().toISOString(),
    updatedAt: new Date().toISOString(),
    providers,                        // { claude: {...}, codex: {...} }
    providerOrder: providerOrder ?? Object.keys(providers),   // the operator's --provider/--fallback order
    activeProvider: null,
    counters: { cycles: 0, commits: 0, consecutiveFailures: 0, emptyAudits: 0, skipped: 0, violations: 0 },
    // What the run spent. `calls` in the provider ring counts callWithRepair
    // invocations and predates this; `usage.byPhase` counts CLI processes,
    // which differ whenever a schema repair fires. Both are kept because the
    // terminal summary has always printed the former.
    usage: { byPhase: {} },
    seen: { done: [], skipped: [], attempts: {} },
    treeHashes: [],
    commits: [],
    activePacket: null,
    terminal: null,                   // { status, reason } once finished
  };
}

export function newProviderRing(names) {
  const ring = {};
  for (const n of names) ring[n] = { status: 'READY', cooldownUntil: null, calls: 0, quotaHits: 0, lastError: null, usage: newUsage() };
  return ring;
}

export class Store {
  constructor(runDir) {
    this.runDir = runDir;
    this.file = path.join(runDir, 'state.json');
    this.state = null;
  }
  load() { this.state = readJson(this.file); return this.state; }
  init(state) { this.state = state; this.save(); return this.state; }
  save() {
    this.state.updatedAt = new Date().toISOString();
    writeJsonAtomic(this.file, this.state);
  }
  transition(next) { this.state.state = next; this.save(); }
}

// ---------------------------------------------------------------- seen-set

export function isSeen(state, fp) {
  return state.seen.done.includes(fp) || state.seen.skipped.some((s) => s.fp === fp);
}

export function attemptsOf(state, fp) {
  return state.seen.attempts[fp] ?? 0;
}

export function noteAttempt(state, fp) {
  state.seen.attempts[fp] = attemptsOf(state, fp) + 1;
  return state.seen.attempts[fp];
}

export function markSkipped(state, fp, reason, detail) {
  if (state.seen.skipped.some((s) => s.fp === fp)) return;
  state.seen.skipped.push({ fp, reason, detail: detail ?? null, at: new Date().toISOString() });
  state.counters.skipped += 1;
}

export function markDone(state, fp) {
  if (!state.seen.done.includes(fp)) state.seen.done.push(fp);
}

// ---------------------------------------------------------------- lock

export class Lock {
  constructor(file) { this.file = file; this.held = false; }

  /** @returns {{ok:true}|{ok:false,holder:object}} */
  acquire() {
    fs.mkdirSync(path.dirname(this.file), { recursive: true });
    const mine = { pid: process.pid, startedAt: new Date().toISOString(), hostname: os.hostname() };
    for (let attempt = 0; attempt < 2; attempt++) {
      try {
        const fd = fs.openSync(this.file, 'wx');
        fs.writeFileSync(fd, JSON.stringify(mine, null, 2));
        fs.closeSync(fd);
        this.held = true;
        return { ok: true };
      } catch (e) {
        if (e.code !== 'EEXIST') throw e;
        const holder = readJson(this.file);
        if (holder && holder.hostname === os.hostname() && !alive(holder.pid)) {
          fs.rmSync(this.file, { force: true });   // stale: the owner is gone
          continue;
        }
        return { ok: false, holder };
      }
    }
    return { ok: false, holder: readJson(this.file) };
  }

  release() {
    if (!this.held) return;
    const holder = readJson(this.file);
    if (holder && holder.pid === process.pid) fs.rmSync(this.file, { force: true });
    this.held = false;
  }
}

function alive(pid) {
  if (typeof pid !== 'number') return false;
  try { process.kill(pid, 0); return true; } catch (e) { return e.code !== 'ESRCH'; }
}
