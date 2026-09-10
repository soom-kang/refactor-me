// scope.mjs — "which folder is this run about".
//
// A run may be narrowed to one or more target directories. The narrowing is
// asymmetric on purpose, and the asymmetry is the whole safety story:
//
//   DISCOVERY is scoped   — the audit only surveys files under a target, so the
//                           model stops proposing work in areas nobody asked about.
//   MODIFICATION is not   — a candidate rooted in the target may still edit its
//                           callers, re-exports and tests outside it, because a
//                           change that stops at a directory boundary is not a
//                           refactor, it is a broken build.
//   VERIFICATION is not   — reachability searches and the validation ladder stay
//                           repository-wide. Narrowing what we look at must never
//                           narrow what we check.
//
// Every path here is a repo-root-relative POSIX path, which is also exactly what
// `git ls-files` and `git status --porcelain` produce — so a target computed
// against the source repository applies unchanged inside the worktree.

import fs from 'node:fs';
import path from 'node:path';

/**
 * What counts as a source file when sizing a scope. Shared by the audit's file
 * inventory (prompts.mjs) and the doctor's "is there anything here at all"
 * precondition, so the two cannot disagree about whether a target is empty.
 */
export const SOURCE_RE = /\.(ts|tsx|js|jsx|mjs|cjs|go|py|rs|java|kt|rb|swift|vue|svelte)$/;
export const VENDORED_RE = /(^|\/)(node_modules|vendor|dist|build)\//;

/** Tracked paths that are real source, i.e. the files an audit could act on. */
export function sourceFiles(tracked) {
  return (tracked ?? []).filter((f) => SOURCE_RE.test(f) && !VENDORED_RE.test(f));
}

/**
 * The single containment rule, borrowed verbatim from collectAreaSkills in
 * prompts.mjs so the two cannot drift. The `+ '/'` is not decoration: without
 * it `app/web` swallows `app/website`.
 */
function under(p, t) {
  return p === t || p.startsWith(`${t}/`);
}

/** No targets means no restriction — every predicate here answers true. */
export function isUnderTarget(p, targets) {
  if (!targets?.length) return true;
  return targets.some((t) => under(p, t));
}

/** Does this candidate/change set have at least one foot inside the scope? */
export function touchesTarget(paths, targets) {
  if (!targets?.length) return true;
  return (paths ?? []).some((p) => isUnderTarget(p, targets));
}

/**
 * Resolve `--target` inputs against the directory the operator typed them in.
 *
 * cwd-relative rather than repo-root-relative because the tool is run from
 * wherever you happen to be: inside `app/web`, `--target src` must mean
 * `app/web/src`. Every rejection names the absolute path we resolved to, so a
 * surprising interpretation is visible rather than silent.
 *
 * Returns { targets, errors }; errors is non-empty only when the caller should
 * abort before doing anything.
 */
export function normalizeTargets(repoRoot, inputs, cwd = process.cwd()) {
  const errors = [];
  const rels = [];

  for (const raw of inputs ?? []) {
    const abs = path.resolve(cwd, raw);
    const rel = path.relative(repoRoot, abs);

    if (rel.startsWith('..') || path.isAbsolute(rel)) {
      errors.push(`--target ${raw}: ${abs} is outside the repository (${repoRoot})`);
      continue;
    }
    if (rel === '') {
      errors.push(`--target ${raw}: that is the repository root, which is the same as not passing --target at all`);
      continue;
    }
    let st = null;
    try { st = fs.statSync(abs); } catch { /* reported below */ }
    if (!st) { errors.push(`--target ${raw}: ${abs} does not exist`); continue; }
    if (!st.isDirectory()) { errors.push(`--target ${raw}: ${abs} is not a directory`); continue; }

    rels.push(rel.split(path.sep).join('/').replace(/\/+$/, ''));
  }

  // Keep only the outermost of any nested pair: `a` already contains `a/b`, and
  // leaving both in would make every containment test do redundant work and
  // every log line read as though two separate areas were in scope.
  const sorted = [...new Set(rels)].sort();
  const targets = sorted.filter((t, i) => !sorted.some((o, j) => j !== i && under(t, o) && o !== t));

  return { targets, errors };
}
