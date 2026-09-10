// git.mjs — every git operation the loop performs.
// Two repos are involved throughout:
//   S = the user's source repository. Its worktree, index and HEAD are NEVER touched.
//   W = a detached worktree outside S. All writes happen here.
// Because a worktree shares S's object database, a commit made in W is instantly
// reachable from S — publishing is one `update-ref`, not a clone/fetch/import.

import { spawnSync } from 'node:child_process';
import fs from 'node:fs';
import path from 'node:path';
import os from 'node:os';
import crypto from 'node:crypto';

export class GitError extends Error {
  constructor(argv, res) {
    const stderr = (res.stderr ?? '').toString().trim();
    super(`git ${argv.join(' ')} -> ${res.status}\n${stderr}`);
    this.name = 'GitError';
    this.argv = argv;
    this.status = res.status;
    this.stderr = stderr;
  }
}

/** Run git, throwing on failure. Never uses a shell. */
export function git(cwd, args, opts = {}) {
  const res = spawnSync('git', args, {
    cwd,
    encoding: 'utf8',
    maxBuffer: 64 * 1024 * 1024,
    input: opts.input,
    env: { ...process.env, GIT_TERMINAL_PROMPT: '0', GIT_OPTIONAL_LOCKS: '0' },
  });
  if (res.error) throw res.error;
  if (res.status !== 0) throw new GitError(args, res);
  return res.stdout;
}

/** Run git, returning {ok,stdout,stderr,status} instead of throwing. */
export function gitTry(cwd, args, opts = {}) {
  const res = spawnSync('git', args, {
    cwd,
    encoding: 'utf8',
    maxBuffer: 64 * 1024 * 1024,
    input: opts.input,
    env: { ...process.env, GIT_TERMINAL_PROMPT: '0', GIT_OPTIONAL_LOCKS: '0' },
  });
  if (res.error) return { ok: false, status: -1, stdout: '', stderr: String(res.error.message) };
  return { ok: res.status === 0, status: res.status, stdout: res.stdout ?? '', stderr: res.stderr ?? '' };
}

const lines = (s) => s.split('\n').map((l) => l.trim()).filter(Boolean);

// ---------------------------------------------------------------- repo facts

export function resolveRepoRoot(startDir) {
  const r = gitTry(startDir, ['rev-parse', '--show-toplevel']);
  if (!r.ok) return null;
  return fs.realpathSync(r.stdout.trim());
}

export function headOid(cwd) {
  return git(cwd, ['rev-parse', '--verify', 'HEAD']).trim();
}

export function currentBranch(cwd) {
  const r = gitTry(cwd, ['symbolic-ref', '--quiet', '--short', 'HEAD']);
  return r.ok ? r.stdout.trim() : null;
}

/** Porcelain status text. Empty string means a clean tree. */
export function statusPorcelain(cwd) {
  return git(cwd, ['status', '--porcelain', '--untracked-files=normal']);
}

/**
 * A stable fingerprint of S's state. The gate re-checks this after every write
 * phase; a change means either a human started working or the agent escaped
 * the worktree — both are HALTED_UNSAFE.
 */
export function sourceFingerprint(cwd) {
  const head = headOid(cwd);
  const st = statusPorcelain(cwd);
  return `${head}:${crypto.createHash('sha1').update(st).digest('hex')}`;
}

export function fileLineCount(cwd, relPath) {
  try {
    const buf = fs.readFileSync(path.join(cwd, relPath));
    if (buf.includes(0)) return null;                       // binary
    let n = 0;
    for (const b of buf) if (b === 10) n++;
    return buf.length > 0 && buf[buf.length - 1] !== 10 ? n + 1 : n;
  } catch { return null; }
}

// ---------------------------------------------------------------- worktree

/**
 * Worktree lives OUTSIDE the repo. Inside, an audit globs into it, `find`
 * recurses into it, and ignore rules get subtle.
 */
export function worktreePath(repoRoot, runId, parentOverride) {
  const hash = crypto.createHash('sha1').update(repoRoot).digest('hex').slice(0, 8);
  const parent = parentOverride && parentOverride.trim() !== ''
    ? parentOverride
    : path.join(os.homedir(), '.cache', 'refactor-me');
  return path.join(parent, `${path.basename(repoRoot)}-${hash}`, runId);
}

/**
 * Nothing the harness writes ever lands inside the worktree. codex's
 * --output-last-message file is produced by the codex process itself, not by
 * the sandboxed model, so it can be written straight into the run directory
 * outside W — verified against codex 0.152.0 in both read-only and
 * workspace-write mode. That keeps the change set the gate reads equal to
 * repository content, with no filtering and no exclude-file surgery.
 */
export function worktreeAdd(repoRoot, wtPath, baseOid) {
  fs.mkdirSync(path.dirname(wtPath), { recursive: true });
  git(repoRoot, ['worktree', 'add', '--detach', wtPath, baseOid]);
  return wtPath;
}

export function worktreeRemove(repoRoot, wtPath) {
  gitTry(repoRoot, ['worktree', 'remove', '--force', wtPath]);
  gitTry(repoRoot, ['worktree', 'prune']);
}

// ---------------------------------------------------------------- change set

/** Tracked modifications + untracked additions, relative to W's HEAD. */
export function changedPaths(wt) {
  const tracked = lines(git(wt, ['diff', '--name-only', '--no-renames', 'HEAD']));
  const untracked = lines(git(wt, ['ls-files', '--others', '--exclude-standard']));
  return [...new Set([...tracked, ...untracked])].sort();
}

/**
 * Remove untracked BINARY files from the worktree and report what was removed.
 *
 * Validation commands produce artifacts — `go build` leaves a Mach-O executable
 * named after the module, bundlers leave chunks — and when a repository has not
 * gitignored them they surface as changed paths and kill an otherwise correct
 * candidate. Observed live: a characterization slice was rejected for "touching
 * app/api/fixtureapi outside the test allowlist", which was a 2.4 MB compiled
 * binary, not an edit.
 *
 * Sweeping only BINARY untracked files is safe rather than convenient: the gate
 * already forbids every binary change outright, so such a file can never be a
 * legitimate refactoring output. A new untracked SOURCE file is left alone,
 * because a split legitimately creates those and the allowlist must judge them.
 *
 * `outputDirs` extends this to TEXT, which the argument above does not cover, so
 * it stands on a different one. A JVM build writes build/test-results/*.xml and
 * target/surefire-reports/*.txt — plain text, therefore invisible to the rule
 * above, and enough to kill every candidate as OUT_OF_SCOPE in a repository that
 * has not gitignored them. Deleting text is narrowed by three things:
 *   1. still only untracked and un-gitignored files — tracked work is untouched;
 *   2. the directories are not a hardcoded guess. They are derived from the
 *      commands discovery actually produced for that area, so we only clear the
 *      output of a build tool we ourselves just ran there;
 *   3. the failure mode is conservative — deleting something wrongly costs a
 *      candidate, never a bad merge, and JVM sources live under src/.
 * Everything here is regenerated by the next ladder run, which is why sweeping
 * twice is harmless.
 */
export function sweepBuildArtifacts(wt, { outputDirs = [] } = {}) {
  const dirs = outputDirs.map((d) => (d.endsWith('/') ? d : `${d}/`));
  const inOutputDir = (rel) => dirs.some((d) => rel.startsWith(d));
  const swept = [];
  for (const rel of lines(git(wt, ['ls-files', '--others', '--exclude-standard']))) {
    const info = binaryInfo(wt, rel);
    let bytes = info?.bytes;
    if (!info) {
      if (!inOutputDir(rel)) continue;                // text outside a build output dir: not ours to remove
      try { bytes = fs.statSync(path.join(wt, rel)).size; } catch { continue; }
    }
    try {
      fs.rmSync(path.join(wt, rel), { force: true });
      swept.push({ path: rel, bytes });
    } catch { /* already gone */ }
  }
  return swept;
}

/** @returns {{bytes:number}|null} null when the file is text, empty or unreadable. */
function binaryInfo(wt, rel) {
  const abs = path.join(wt, rel);
  let fd;
  try {
    const st = fs.statSync(abs);
    if (!st.isFile() || st.size === 0) return null;
    fd = fs.openSync(abs, 'r');
    const buf = Buffer.alloc(Math.min(8192, st.size));
    fs.readSync(fd, buf, 0, buf.length, 0);
    return buf.includes(0) ? { bytes: st.size } : null;
  } catch {
    return null;
  } finally {
    if (fd !== undefined) { try { fs.closeSync(fd); } catch { /* closed */ } }
  }
}

export function deletedPaths(wt) {
  return lines(git(wt, ['diff', '--name-only', '--diff-filter=D', 'HEAD'])).sort();
}

/**
 * @returns {{insertions:number, deletions:number, binary:string[], perFile:Map<string,{ins:number,del:number}>}}
 * A binary file shows as `-\t-\t<path>`; we surface those regardless of extension.
 */
export function numstat(wt, ref = 'HEAD') {
  const out = git(wt, ['diff', '--numstat', '--no-renames', ref]);
  let insertions = 0, deletions = 0;
  const binary = [];
  const perFile = new Map();
  for (const raw of out.split('\n')) {
    if (!raw.trim()) continue;
    const [a, d, ...rest] = raw.split('\t');
    const file = rest.join('\t');
    if (a === '-' || d === '-') { binary.push(file); perFile.set(file, { ins: 0, del: 0, binary: true }); continue; }
    const ins = Number(a) || 0, del = Number(d) || 0;
    insertions += ins; deletions += del;
    perFile.set(file, { ins, del, binary: false });
  }
  return { insertions, deletions, binary, perFile };
}

/** Diff of a single file, used for test-integrity inspection of added lines. */
export function addedLinesFor(wt, file) {
  const out = gitTry(wt, ['diff', '--unified=0', 'HEAD', '--', file]).stdout;
  return out.split('\n').filter((l) => l.startsWith('+') && !l.startsWith('+++')).map((l) => l.slice(1));
}

export function diffPatch(wt, ref = 'HEAD') {
  return gitTry(wt, ['diff', ref]).stdout;
}

/**
 * The diff the reviewer reads. Untracked files are included so newly created
 * source is visible, but BINARY untracked files are excluded by name only.
 *
 * The validation ladder re-creates build artifacts after the gate swept them —
 * `go build` runs during validation and puts the executable back — so by review
 * time an artifact can be present again. Observed live: a reviewer returned
 * BLOCKER / CANNOT_DETERMINE because "the diff adds a compiled binary build
 * artifact", which was true of the diff it was shown and had nothing to do with
 * the candidate. Naming them without their contents keeps the reviewer informed
 * and out of a 2 MB byte dump.
 */
export function diffForReview(wt, ref = 'HEAD') {
  const parts = [gitTry(wt, ['diff', ref]).stdout];
  const skipped = [];
  for (const f of lines(git(wt, ['ls-files', '--others', '--exclude-standard']))) {
    if (binaryInfo(wt, f)) { skipped.push(f); continue; }
    parts.push(gitTry(wt, ['diff', '--no-index', '--', '/dev/null', f]).stdout);
  }
  if (skipped.length) {
    parts.push(`\n# (${skipped.length} untracked binary file(s) omitted as build output, not part of this change: ${skipped.join(', ')})\n`);
  }
  return parts.join('');
}

// ---------------------------------------------------------------- staging & commit

/**
 * Idempotent by necessity: the change set is staged once so it can be measured
 * (untracked files are invisible to `git diff --numstat HEAD` otherwise) and
 * again so the commit can be proved identical to what was validated. A path
 * already staged as a deletion exists in neither the worktree nor the index, and
 * `git add` rejects it with "did not match any files".
 */
export function stageExact(wt, paths) {
  if (paths.length === 0) return;
  const alreadyDeleted = new Set(
    lines(git(wt, ['diff', '--cached', '--name-only', '--diff-filter=D', 'HEAD'])));
  const pending = paths.filter((p) => !alreadyDeleted.has(p));
  if (pending.length === 0) return;
  git(wt, ['add', '-A', '--', ...pending]);
}

/** The prospective tree, used for the thrash / revert check. */
export function writeTree(wt) {
  return git(wt, ['write-tree']).trim();
}

export function commit(wt, messageFile) {
  git(wt, [
    '-c', 'user.name=refactor-me',
    '-c', 'user.email=refactor-me@local',
    'commit', '--no-verify', '-F', messageFile,
  ]);
  return headOid(wt);
}

/**
 * Publish into S by compare-and-swap on the branch ref. Objects are already in
 * S's odb because W shares it. Never touches S's index, HEAD or working tree.
 * `prevOid` of null means "the ref must not exist yet".
 */
export function updateRefCas(repoRoot, ref, newOid, prevOid) {
  const args = ['update-ref', ref, newOid];
  args.push(prevOid ?? '');
  const r = gitTry(repoRoot, args);
  if (!r.ok) throw new GitError(args, { status: r.status, stderr: r.stderr });
  return newOid;
}

export function refExists(repoRoot, ref) {
  return gitTry(repoRoot, ['rev-parse', '--verify', '--quiet', ref]).ok;
}

// ---------------------------------------------------------------- rollback

/**
 * Undo an abandoned candidate. `clean -ffd` deliberately omits -x so hydrated
 * gitignored build inputs (node_modules, .env.local) survive.
 * @returns {{clean:boolean, head:string, residue:string}}
 */
export function rollback(wt, preOid) {
  gitTry(wt, ['reset', '--hard', preOid]);
  gitTry(wt, ['clean', '-ffd']);
  const residue = statusPorcelain(wt).trim();
  return { clean: residue === '', head: headOid(wt), residue };
}

export function checkoutDetached(wt, oid) {
  git(wt, ['checkout', '--detach', oid]);
}

export function logOneline(repoRoot, range, limit = 50) {
  return gitTry(repoRoot, ['log', '--oneline', `-${limit}`, range]).stdout;
}

export function trailerFingerprints(repoRoot, range) {
  const out = gitTry(repoRoot, ['log', '--format=%(trailers:key=Refactor-Fingerprint,valueonly)', range]).stdout;
  return lines(out);
}
