// gate.mjs — the diff allowlist gate.
//
// This is where the design differs from its predecessor: the heavy version asked
// the agent to PROVE the change was safe; this one MEASURES it. No LLM judgment
// participates here. Inputs are frozen before EXECUTE and re-read from disk.
//
// Checks run in a fixed order, cheapest and most dangerous first, first failure
// wins. Only steps 1-2 can produce HALTED_UNSAFE; everything else is a
// recoverable candidate abandonment.

import * as G from './git.mjs';
import { isUnderTarget } from './scope.mjs';

// ---------------------------------------------------------------- glob

/**
 * Repo-relative POSIX glob -> RegExp.
 *   double-star + slash  matches at any depth (including the root)
 *   *     matches within one segment
 *   a pattern with no "/" matches that basename at any depth (gitignore-like)
 */
export function globToRegex(glob) {
  const anchored = glob.includes('/') ? glob : `**/${glob}`;
  let re = '^';
  let i = 0;
  while (i < anchored.length) {
    const c = anchored[i];
    if (c === '*') {
      if (anchored[i + 1] === '*') {
        i += 2;
        if (anchored[i] === '/') { i += 1; re += '(?:[^/]+/)*'; }
        else re += '.*';
      } else { i += 1; re += '[^/]*'; }
    } else if (c === '?') { i += 1; re += '[^/]'; }
    else if ('.+^${}()|[]\\'.includes(c)) { re += '\\' + c; i += 1; }
    else { re += c; i += 1; }
  }
  return new RegExp(re + '$');
}

export function matchesAny(p, regexes) {
  return regexes.some((r) => r.test(p));
}

/**
 * Never removable by configuration — config may only ADD to this list.
 * Anything whose modification would change behavior, dependencies, generated
 * output, or the evidence the loop relies on.
 */
export const FORBIDDEN_GLOBS = [
  '.git', '.git/**',
  // dependency + manifest surface: if deps cannot change, they need no snapshotting
  '**/package.json', '**/package-lock.json', '**/pnpm-lock.yaml', '**/yarn.lock',
  '**/bun.lock', '**/bun.lockb', '**/npm-shrinkwrap.json',
  '**/go.mod', '**/go.sum', '**/Cargo.toml', '**/Cargo.lock',
  '**/pyproject.toml', '**/poetry.lock', '**/uv.lock', '**/Pipfile.lock', '**/requirements*.txt',
  '**/Gemfile', '**/Gemfile.lock', '**/composer.json', '**/composer.lock',
  '**/*.lock',
  // JVM. Wildcarded rather than named one by one because the convention is to
  // spread declarations across files — versions.gradle, dependencies.gradle,
  // build-logic/*.gradle.kts — and any of them can move a dependency. Note
  // `gradle.lockfile` does NOT match '**/*.lock' above.
  '**/*.gradle', '**/*.gradle.kts', '**/gradle.properties',
  '**/gradle/wrapper/**', '**/gradlew', '**/gradlew.bat',
  '**/gradle/libs.versions.toml', '**/gradle.lockfile', '**/gradle/dependency-locks/**',
  '**/pom.xml', '**/.mvn/**', '**/mvnw', '**/mvnw.cmd',
  // oracles: updating these to accept new output is the classic silent regression
  '**/__snapshots__/**', '**/*.snap', '**/*.ambr', '**/__image_snapshots__/**',
  '**/testdata/**', '**/golden/**', '**/*.golden', '**/fixtures/**',
  '**/src/test/resources/**',            // the JVM convention for the same thing
  // assets
  '*.png', '*.jpg', '*.jpeg', '*.gif', '*.webp', '*.svg', '*.ico', '*.pdf',
  '*.woff', '*.woff2', '*.wasm', '*.zip', '*.tar', '*.gz',
  // deployment + infrastructure
  '.github/**', '.gitlab-ci.yml', 'Jenkinsfile', '**/Dockerfile*',
  '**/compose*.yml', '**/compose*.yaml', '**/*.tf', '**/*.tfstate',
  '**/migrations/**', '**/db/migrations/**',
  // secrets
  '.env', '.env.*', '**/.env', '**/.env.*',
  // the harness and its instructions
  '.refactor/**', '.agents/**', '.claude/**',
  'AGENTS.md', 'CLAUDE.md', '**/AGENTS.md', '**/CLAUDE.md',
  // published contracts and generated code
  '**/openapi.yaml', '**/openapi.yml', '**/openapi.json',
  '**/*.proto', '**/schema.graphql',
  '**/generated/**', '**/*.gen.go', '**/*_gen.go', '**/*.pb.go',
  'LICENSE',
];

export const TEST_GLOBS = [
  '**/*.test.*', '**/*.spec.*', '**/test/**', '**/tests/**', '**/__tests__/**',
  '**/*_test.go', '**/test_*.py', '**/*_test.py', '**/*_test.rs',
  // JVM. src/test/** is already covered by '**/test/**', but the suffix
  // conventions are not, and a source set can be named src/integrationTest.
  // '**/*IT.java' also catches something like SPLIT.java — the consequence is
  // that lines cannot be deleted from it, which errs toward refusing work
  // rather than toward permitting a weakened test.
  '**/src/*Test/**', '**/src/*test/**',   // globToRegex has no character classes
  '**/*Test.java', '**/*Tests.java', '**/*TestCase.java', '**/*IT.java',
  '**/*Test.kt', '**/*Tests.kt', '**/*Spec.kt', '**/*Spec.groovy',
];

/** Tokens that weaken a test without deleting it. */
export const WEAKENING_RE =
  /\.skip\(|\.only\(|\bxit\(|\bxdescribe\(|\bt\.Skip\(|@pytest\.mark\.skip|#\[ignore\]|\bt\.SkipNow\(|@Disabled\b|@Ignore\b|\bassume(?:True|False|That|NotNull)\s*\(|\benabled\s*=\s*false\b/;

export function isTestPath(p, extra = []) {
  return matchesAny(p, [...TEST_GLOBS, ...extra].map(globToRegex));
}

// ---------------------------------------------------------------- net size

/**
 * A removal that does not remove, or a consolidation that grows the codebase,
 * did not do what its category claims.
 */
export function netSizeRule(kind, { insertions, deletions, originalLines, primaryFileLines }) {
  const net = insertions - deletions;
  switch (kind) {
    case 'DEAD_CODE':
    case 'COMPATIBILITY_REMOVAL':
      return net <= 5
        ? { ok: true, detail: `net ${net} (limit +5)` }
        : { ok: false, detail: `net ${net} exceeds +5; a removal must remove` };
    case 'DEDUPLICATION':
      return net <= 0
        ? { ok: true, detail: `net ${net} (limit 0)` }
        : { ok: false, detail: `net ${net} > 0; consolidation that grows the codebase is not consolidation` };
    case 'LARGE_COMPONENT_SPLIT': {
      const budget = Math.max(20, Math.round((originalLines ?? 0) * 0.10));
      if (Math.abs(net) > budget) {
        return { ok: false, detail: `net ${net} exceeds ±${budget} (10% of ${originalLines}); a split redistributes, it does not rewrite` };
      }
      if (primaryFileLines != null && primaryFileLines >= 500) {
        return { ok: false, detail: `primary file still ${primaryFileLines} lines; the stated goal was not achieved (SCOPE_DRIFT)` };
      }
      return { ok: true, detail: `net ${net} within ±${budget}, primary now ${primaryFileLines ?? '?'} lines` };
    }
    default:
      return { ok: true, detail: `no net-size rule for ${kind}` };
  }
}

/**
 * Insertions + deletions is a good proxy for blast radius everywhere EXCEPT a
 * split, where it is actively wrong: moving N lines out of one file and into
 * another necessarily touches about 2N lines, so a flat 600-line cap makes any
 * file over ~300 lines unsplittable — precisely the files this loop exists to
 * split. Splits get a budget derived from the file they are splitting; the
 * net-size rule and the file-count cap remain the real bounds on their scope.
 */
export function changedLineBudget(packet, policy) {
  const flat = policy?.max_changed_lines ?? 600;
  if (packet?.category !== 'LARGE_COMPONENT_SPLIT') return flat;
  const original = packet.original_lines ?? 0;
  return Math.max(flat, Math.ceil(original * 2.5));
}

// ---------------------------------------------------------------- facts

/**
 * Everything the checks need, gathered in one pass so the pure logic below can
 * be unit-tested without a repository.
 */
export function collectFacts({ wt, repoRoot, preOid, baseOid, sourceFp, packet }) {
  const changed = G.changedPaths(wt);
  // Stage BEFORE measuring. `git diff --numstat HEAD` does not see untracked
  // files, so an unstaged change set reports the deletions from a split's source
  // file and none of the insertions in the files it created — making every
  // successful split look like a mass deletion and fail the net-size rule.
  if (changed.length > 0) G.stageExact(wt, changed);
  const deleted = G.deletedPaths(wt);
  const ns = G.numstat(wt);
  const addedByFile = {};
  for (const f of changed) {
    if (isTestPath(f)) addedByFile[f] = G.addedLinesFor(wt, f);
  }
  let primaryFileLines = null;
  const primary = packet?.allowlist?.[0];
  if (primary) primaryFileLines = G.fileLineCount(wt, primary);

  return {
    worktreeHead: G.headOid(wt),
    sourceHead: G.headOid(repoRoot),
    sourceFpNow: G.sourceFingerprint(repoRoot),
    preOid,
    baseOid,
    sourceFpBefore: sourceFp,
    changed,
    deleted,
    insertions: ns.insertions,
    deletions: ns.deletions,
    binary: ns.binary,
    perFile: ns.perFile,
    addedByFile,
    primaryFileLines,
  };
}

// ---------------------------------------------------------------- checks

/** Steps in runChecks, for honest "n/N" labelling in the live log. */
export const GATE_CHECK_COUNT = 11;

const ok = (id, detail) => ({ id, ok: true, detail });
const bad = (id, code, detail, halt = false) => ({ id, ok: false, code, detail, halt });

/**
 * Pure. Returns { verdict, checks, violation }.
 *   verdict: 'PASS' | 'NO_OP' | 'VIOLATION' | 'HALT'
 * The caller runs the validation ladder (step 12) only when verdict === 'PASS'.
 */
export function runChecks(facts, packet, policy, state, targets = []) {
  const checks = [];
  const emit = (c) => { checks.push(c); return c.ok; };
  const stop = (c) => ({ verdict: c.halt ? 'HALT' : 'VIOLATION', checks, violation: c });

  // 1 — did the agent commit, reset or rebase behind our back?
  {
    const c = facts.worktreeHead === facts.preOid
      ? ok('WORKTREE_HEAD_STABLE', `HEAD ${facts.preOid.slice(0, 7)}`)
      : bad('WORKTREE_HEAD_STABLE', 'WORKTREE_HEAD_MOVED',
            `expected ${facts.preOid.slice(0, 7)}, found ${facts.worktreeHead.slice(0, 7)}`, true);
    if (!emit(c)) return stop(c);
  }

  // 2 — did anything escape the worktree, or did a human start working?
  {
    const same = facts.sourceHead === facts.baseOid && facts.sourceFpNow === facts.sourceFpBefore;
    const c = same
      ? ok('SOURCE_UNTOUCHED', `HEAD ${facts.baseOid.slice(0, 7)}, status unchanged`)
      : bad('SOURCE_UNTOUCHED', 'SOURCE_MUTATED',
            `source repository changed during the write phase (head ${facts.sourceHead.slice(0, 7)})`, true);
    if (!emit(c)) return stop(c);
  }

  // 3 — the change set itself
  emit(ok('CHANGED_SET', `${facts.changed.length} path(s), ${facts.deleted.length} deleted`));

  // 4 — nothing happened
  if (facts.changed.length === 0) {
    checks.push(ok('NON_EMPTY', 'no changes produced'));
    return { verdict: 'NO_OP', checks, violation: null };
  }
  emit(ok('NON_EMPTY', `${facts.changed.length} changed`));

  // 5 — forbidden paths. Highest severity: these can never be in scope.
  {
    const forb = [...FORBIDDEN_GLOBS, ...(policy?.extra_forbidden_globs ?? [])].map(globToRegex);
    const hits = facts.changed.filter((p) => matchesAny(p, forb));
    const binHits = facts.binary ?? [];
    if (hits.length > 0) {
      const c = bad('FORBIDDEN_PATH', 'FORBIDDEN_PATH', `forbidden: ${hits.join(', ')}`);
      checks.push(c); return stop(c);
    }
    if (binHits.length > 0) {
      const c = bad('FORBIDDEN_PATH', 'FORBIDDEN_BINARY', `binary content changed: ${binHits.join(', ')}`);
      checks.push(c); return stop(c);
    }
    emit(ok('FORBIDDEN_PATH', `0 hits across ${facts.changed.length} changed path(s)`));
  }

  // 6 — the allowlist. A split may add siblings next to an allowlisted file.
  {
    const allow = new Set(packet.allowlist ?? []);
    const allowDirs = new Set((packet.allowlist ?? []).map((p) => p.replace(/[^/]*$/, '')));
    const exts = new Set((packet.allowlist ?? []).map((p) => (p.match(/\.[^./]+$/) ?? [''])[0]));
    const outside = [];
    const perPath = [];
    for (const p of facts.changed) {
      if (allow.has(p)) { perPath.push(`${p} ok`); continue; }
      const sameDir = allowDirs.has(p.replace(/[^/]*$/, ''));
      const sameExt = exts.has((p.match(/\.[^./]+$/) ?? [''])[0]);
      if (packet.category === 'LARGE_COMPONENT_SPLIT' && sameDir && sameExt) {
        perPath.push(`${p} ok (split sibling)`); continue;
      }
      outside.push(p); perPath.push(`${p} OUTSIDE`);
    }
    if (outside.length > 0) {
      const c = bad('ALLOWLIST', 'OUT_OF_SCOPE', `outside allowlist: ${outside.join(', ')}`);
      c.paths = outside; checks.push(c); return stop(c);
    }
    emit({ ...ok('ALLOWLIST', perPath.join(', ')), perPath });
  }

  // 7 — the run's target folder, when one was named. The allowlist above is the
  // model's own account of scope; this is the only check that reads the actual
  // change set against what the operator asked for. One changed path inside is
  // enough: editing the callers outside the target is expected and allowed.
  {
    if (!targets?.length) {
      emit(ok('TARGET_SCOPE', 'no target restriction'));
    } else {
      const inside = facts.changed.filter((p) => isUnderTarget(p, targets));
      const c = inside.length > 0
        ? ok('TARGET_SCOPE', `${inside.length}/${facts.changed.length} changed path(s) under ${targets.join(', ')}`)
        : bad('TARGET_SCOPE', 'TARGET_SCOPE_EMPTY',
              `nothing under ${targets.join(', ')} was changed: ${facts.changed.join(', ')}`);
      if (!emit(c)) return stop(c);
    }
  }

  // 8 — blast radius caps
  {
    const maxFiles = policy?.max_files_per_candidate ?? 8;
    const maxLines = changedLineBudget(packet, policy);
    const total = facts.insertions + facts.deletions;
    const c = (facts.changed.length <= maxFiles && total <= maxLines)
      ? ok('SIZE_CAPS', `${facts.changed.length} files, ${total} lines (caps ${maxFiles} / ${maxLines})`)
      : bad('SIZE_CAPS', 'TOO_LARGE', `${facts.changed.length} files / ${total} lines exceeds ${maxFiles} / ${maxLines}`);
    if (!emit(c)) return stop(c);
  }

  // 9 — did the change do what its category claims?
  {
    const r = netSizeRule(packet.category, {
      insertions: facts.insertions,
      deletions: facts.deletions,
      originalLines: packet.original_lines ?? null,
      primaryFileLines: facts.primaryFileLines,
    });
    const c = r.ok ? ok('NET_SIZE', r.detail) : bad('NET_SIZE', 'NET_SIZE_RULE', r.detail);
    if (!emit(c)) return stop(c);
  }

  // 10 — tests are the oracle; they may grow, never shrink or soften
  {
    const problems = [];
    for (const p of facts.deleted) if (isTestPath(p)) problems.push(`test file deleted: ${p}`);
    for (const p of facts.changed) {
      if (!isTestPath(p)) continue;
      const st = facts.perFile.get(p);
      if (st && st.del > 0) problems.push(`${p} removes ${st.del} test line(s)`);
      for (const line of facts.addedByFile[p] ?? []) {
        if (WEAKENING_RE.test(line)) { problems.push(`${p} adds a skip/only marker`); break; }
      }
    }
    const c = problems.length === 0
      ? ok('TEST_INTEGRITY', `${facts.changed.filter((p) => isTestPath(p)).length} test file(s) touched, none weakened`)
      : bad('TEST_INTEGRITY', 'TEST_WEAKENED', problems.join('; '));
    if (!emit(c)) return stop(c);
  }

  // 11 — thrash: an identical tree means a no-op or an exact revert of prior work
  {
    const tree = facts.prospectiveTree;
    const seen = state?.treeHashes ?? [];
    const c = (tree && seen.includes(tree))
      ? bad('THRASH', 'THRASH_REVERT', `tree ${tree.slice(0, 7)} already produced earlier in this run`)
      : ok('THRASH', `tree ${(tree ?? 'n/a').slice(0, 7)} not seen before`);
    if (!emit(c)) return stop(c);
  }

  return { verdict: 'PASS', checks, violation: null };
}

/**
 * Abandon a candidate: keep the evidence, then restore W exactly.
 * A residue after `reset --hard` + `clean -ffd` is REPO_UNSAFE, not a retry.
 */
export function rollbackCandidate(wt, preOid, { savePatch } = {}) {
  if (savePatch) {
    try { savePatch(G.diffPatch(wt), G.changedPaths(wt)); } catch { /* evidence is best-effort */ }
  }
  const r = G.rollback(wt, preOid);
  return {
    clean: r.clean && r.head === preOid,
    head: r.head,
    residue: r.residue,
  };
}
