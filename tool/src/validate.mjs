// validate.mjs — area detection, validation-command discovery, the baseline, and
// the differential comparison that lets the loop run in a repository whose
// checks are already failing (which is most real repositories).
//
// Three tiers, never more. Integration, e2e, -race, docker-compose and migration
// suites are NEVER executed by the loop: they need services and secrets, and
// they are what turns an unattended run into a support ticket. They are listed
// in the report under "run these before merging" instead.

import { spawn } from 'node:child_process';
import fs from 'node:fs';
import path from 'node:path';

export const TIER = { FAST: 'T1', TEST: 'T2', BUILD: 'T3' };

const TIER_TIMEOUT_MS = { T1: 300_000, T2: 900_000, T3: 900_000 };

const PRUNE = new Set([
  'node_modules', 'vendor', '.git', 'dist', 'build', '.next', 'target',
  '.venv', 'venv', 'coverage', '.refactor', '.agents', '.claude',
]);

/** A test script we must never run unattended. */
const HEAVY_RE = /playwright|cypress|--watch\b|\be2e\b|vitest\s+--ui|testcontainers|docker|compose|psql|migrate|-race\b/i;

// ---------------------------------------------------------------- areas

const MARKERS = ['package.json', 'go.mod', 'pyproject.toml', 'Cargo.toml', 'Gemfile', 'Makefile'];

/**
 * JVM build roots. Held apart from MARKERS because they nest: a Gradle or Maven
 * MODULE carries the same file as the build that owns it, and registering the
 * module as an area is not a smaller mistake than missing it — it is a silent
 * one. areasForPaths maps a changed file to the DEEPEST matching area, so a
 * registered `svc/app` would take the file, the area holding the actual command
 * (`svc`) would fall out of scope, and the JVM checks would simply not run
 * while the report said validation passed.
 */
const JVM_ROOTS = ['settings.gradle', 'settings.gradle.kts', 'build.gradle', 'build.gradle.kts', 'pom.xml'];

/**
 * True when an ancestor, up to and excluding `root`, carries one of the same
 * markers.
 *
 * The early return is load-bearing. Without it the root area — the commonest
 * case, a repository that IS the Gradle or Maven project — started the walk at
 * `dirname(root)`, i.e. OUTSIDE the repository, and never met the `stop`
 * comparison, so it climbed to the filesystem root. One stray pom.xml in a
 * parent directory (a CI workspace, a home directory) then silenced the whole
 * project's validation with no message at all.
 */
function outrankedByAncestor(root, dir, markers) {
  const stop = path.resolve(root);
  if (path.resolve(dir) === stop) return false;
  let cur = path.dirname(dir);
  for (;;) {
    if (markers.some((m) => fs.existsSync(path.join(cur, m)))) return true;
    if (path.resolve(cur) === stop) return false;
    const up = path.dirname(cur);
    if (up === cur) return false;
    cur = up;
  }
}

/** Any directory at depth <= 3 carrying a project marker, plus the root. */
export function detectAreas(root, maxDepth = 3) {
  const found = new Set(['.']);
  const walk = (dir, depth) => {
    if (depth > maxDepth) return;
    let entries;
    try { entries = fs.readdirSync(dir, { withFileTypes: true }); } catch { return; }
    for (const e of entries) {
      if (!e.isDirectory() || PRUNE.has(e.name) || e.name.startsWith('.')) continue;
      const abs = path.join(dir, e.name);
      const rel = path.relative(root, abs);
      if (MARKERS.some((m) => fs.existsSync(path.join(abs, m)))) found.add(rel);
      else if (JVM_ROOTS.some((m) => fs.existsSync(path.join(abs, m)))
        && !outrankedByAncestor(root, abs, JVM_ROOTS)) found.add(rel);
      walk(abs, depth + 1);
    }
  };
  walk(root, 1);
  return [...found].sort();
}

// ---------------------------------------------------------------- discovery

function pkgRunner(dir) {
  if (fs.existsSync(path.join(dir, 'bun.lock')) || fs.existsSync(path.join(dir, 'bun.lockb'))) return 'bun';
  if (fs.existsSync(path.join(dir, 'pnpm-lock.yaml'))) return 'pnpm';
  if (fs.existsSync(path.join(dir, 'yarn.lock'))) return 'yarn';
  return 'npm';
}

/**
 * Python has no lockfile-to-runner convention as tidy as npm's, but the two
 * lockfiles that DO exist name a runner that provisions the environment itself.
 * Without one we fall back to the bare binary, i.e. "whatever is on PATH".
 */
function pyRunner(dir) {
  if (fs.existsSync(path.join(dir, 'uv.lock'))) return ['uv', 'run', '--'];
  if (fs.existsSync(path.join(dir, 'poetry.lock'))) return ['poetry', 'run'];
  return [];
}

/**
 * A `[tool.ruff]` block is CONFIGURATION, not availability: plenty of projects
 * configure a linter they never install. Only a dependency declaration — the
 * tool's name inside a quoted requirement string — means the runner will
 * actually have the binary.
 */
const pyDeclared = (text, tool) => new RegExp(`["']${tool}\\b`).test(text);

function readJsonSafe(f) {
  try { return JSON.parse(fs.readFileSync(f, 'utf8')); } catch { return null; }
}

function pickScript(scripts, names) {
  for (const n of names) if (typeof scripts[n] === 'string') return n;
  return null;
}

/**
 * Resolve the T1/T2/T3 commands for one area. Returns an array of command specs;
 * an area contributing nothing is simply skipped.
 */
export function discoverArea(root, area) {
  const dir = path.join(root, area);
  const out = [];
  // `family` is in the id because the id is the key runLadder matches a result
  // against its baseline. Without it, an area holding both a package.json and a
  // go.mod (or a Gradle build) minted two `.:T2:test` entries, and the Map in
  // runLadder silently kept the last one — so the first command was compared to
  // the OTHER command's baseline. `name` stays clean: it is what the report and
  // the agent prompt show a person.
  const add = (family, tier, name, argv, source) => {
    const id = `${area}:${tier}:${family}:${name}`;
    if (out.some((c) => c.id === id)) return;
    out.push({ id, area, tier, family, name, argv, cwd: area, timeoutMs: TIER_TIMEOUT_MS[tier], source });
  };

  // 1 — package.json scripts: the most precise signal available.
  const pkg = readJsonSafe(path.join(dir, 'package.json'));
  if (pkg && pkg.scripts) {
    const runner = pkgRunner(dir);
    const s = pkg.scripts;
    const tc = pickScript(s, ['typecheck', 'type-check', 'tsc']);
    if (tc) add('npm', TIER.FAST, 'typecheck', [runner, 'run', tc], `package.json scripts.${tc}`);
    const lint = pickScript(s, ['lint']);
    if (lint && !HEAVY_RE.test(s[lint])) add('npm', TIER.FAST, 'lint', [runner, 'run', lint], `package.json scripts.${lint}`);
    const test = pickScript(s, ['test']);
    if (test) {
      if (HEAVY_RE.test(s[test])) {
        out.push({ id: `${area}:SKIPPED:npm:test`, area, tier: 'SKIPPED', family: 'npm', name: 'test',
          argv: [runner, 'run', test], cwd: area, timeoutMs: 0,
          source: `package.json scripts.${test}`, reason: 'looks like an e2e/browser/service suite' });
      } else {
        add('npm', TIER.TEST, 'test', [runner, 'run', test], `package.json scripts.${test}`);
      }
    }
    const build = pickScript(s, ['build']);
    if (build && !HEAVY_RE.test(s[build])) add('npm', TIER.BUILD, 'build', [runner, 'run', build], `package.json scripts.${build}`);
  }

  // 2 — Go. Never -race, never integration build tags.
  if (fs.existsSync(path.join(dir, 'go.mod'))) {
    add('go', TIER.FAST, 'vet', ['go', 'vet', './...'], 'go.mod');
    add('go', TIER.TEST, 'test', ['go', 'test', '-count=1', './...'], 'go.mod');
    add('go', TIER.BUILD, 'build', ['go', 'build', './...'], 'go.mod');
  }

  // 3 — Python.
  const pyproject = fs.existsSync(path.join(dir, 'pyproject.toml'))
    ? fs.readFileSync(path.join(dir, 'pyproject.toml'), 'utf8') : null;
  if (pyproject) {
    const runner = pyRunner(dir);
    // A declared dependency goes through the runner, which provisions it. A tool
    // that is merely configured gets the bare argv and stands or falls on PATH —
    // and a missing binary is caught as UNRUNNABLE at baseline, not mistaken for
    // a failing check.
    const py = (tier, name, argv, tool) => {
      if (!new RegExp(`\\b${tool}\\b`).test(pyproject)) return;
      const declared = pyDeclared(pyproject, tool);
      add('py', tier, name, declared ? [...runner, ...argv] : argv,
        `pyproject.toml ${declared ? `dependency ${tool}` : `[${tool}]`}`);
    };
    py(TIER.FAST, 'lint', ['ruff', 'check', '.'], 'ruff');
    py(TIER.FAST, 'typecheck', ['mypy', '.'], 'mypy');
    py(TIER.TEST, 'test', ['pytest', '-q', '-m', 'not integration'], 'pytest');
  }

  // 4 — Rust.
  if (fs.existsSync(path.join(dir, 'Cargo.toml'))) {
    add('rust', TIER.FAST, 'check', ['cargo', 'check'], 'Cargo.toml');
    add('rust', TIER.TEST, 'test', ['cargo', 'test', '--lib'], 'Cargo.toml');
  }

  // 5 — Gradle. testClasses compiles main and test sources without running
  // anything, which is the same shape as `cargo check` and `go vet`, and is a
  // strict subset of what `test` does — so the cheap check really is cheaper.
  // No T3: `assemble`/`package` would recompile what T2 already compiled, once
  // per candidate, and it is the most expensive command in the ladder.
  // No --offline (an unresolvable dependency fails with no file:line, which is
  // an empty signature, i.e. a gate that validates nothing) and no --no-daemon
  // (the daemon does not affect results, and it inherits the process group the
  // timeout kills).
  const isGradleRoot = ['build.gradle', 'build.gradle.kts', 'settings.gradle', 'settings.gradle.kts']
    .some((f) => fs.existsSync(path.join(dir, f)));
  if (isGradleRoot && !outrankedByAncestor(root, dir, JVM_ROOTS)) {
    if (JVM_PLUGIN_RE.test(gradleScripts(dir))) {
      const g = wrapper(dir, 'gradlew', 'gradle');
      add('gradle', TIER.FAST, 'typecheck', [g, 'testClasses', '--console=plain'], 'build.gradle (JVM plugin)');
      add('gradle', TIER.TEST, 'test', [g, 'test', '--console=plain'], 'build.gradle (JVM plugin)');
    }
    // No plugin evidence: contribute nothing. If that leaves the repository with
    // no command at all, ABORTED is the honest answer and .refactor/commands.json
    // is the way to say otherwise.
  }

  // 6 — Maven. No plugin sniffing needed: test-compile and test are lifecycle
  // phases the tool defines, so they exist in every pom.xml. -B keeps the output
  // free of progress animations that would pollute a signature; -ntp is not used
  // because it is rejected outright by Maven older than 3.6.1.
  if (fs.existsSync(path.join(dir, 'pom.xml')) && !outrankedByAncestor(root, dir, JVM_ROOTS)) {
    const m = wrapper(dir, 'mvnw', 'mvn');
    add('maven', TIER.FAST, 'typecheck', [m, '-B', 'test-compile'], 'pom.xml');
    add('maven', TIER.TEST, 'test', [m, '-B', 'test'], 'pom.xml');
  }

  // 7 — Makefile, ranked last: phony targets are usually the heavy composites,
  // so a native command of the SAME NAME wins. The deduplication is per-name and
  // never global: a `[tool.ruff]` block contributing one T1 lint must not hide a
  // Makefile that owns the only test suite in the repository.
  const mk = fs.existsSync(path.join(dir, 'Makefile')) ? fs.readFileSync(path.join(dir, 'Makefile'), 'utf8') : null;
  if (mk) {
    const taken = new Set(out.map((c) => c.name));
    for (const t of ['lint', 'typecheck', 'check', 'test', 'build']) {
      if (taken.has(t)) continue;
      const m = new RegExp(`^${t}:`, 'm').exec(mk);
      if (!m) continue;
      const body = mk.slice(m.index).split('\n').slice(1).join('\n').split(/\n(?=\S)/)[0] ?? '';
      if (HEAVY_RE.test(body)) continue;
      const tier = t === 'test' ? TIER.TEST : t === 'build' ? TIER.BUILD : TIER.FAST;
      // cwd is already the area — `-C <abspath>` would bake the discovery root
      // into a command spec that outlives the worktree it was discovered in.
      add('make', tier, t, ['make', t], `Makefile target ${t}`);
    }
  }

  return out;
}

/**
 * `./gradlew` when the wrapper is present AND executable, otherwise the PATH
 * binary. The executable bit matters: a wrapper committed from Windows arrives
 * mode 644, and spawning it fails with EACCES — which would take the whole JVM
 * area out as UNRUNNABLE, when falling back to `gradle` would have worked.
 *
 * This is a file attribute inside the repository, not a PATH lookup: whether
 * `gradle` itself exists is still decided at baseline, by trying it.
 *
 * The relative `./gradlew` is deliberate. spawn() chdirs to cwd before exec, so
 * it resolves against the area — and an absolute path would bake the discovery
 * root into a spec that outlives the worktree it was discovered in.
 */
function wrapper(dir, name, fallback) {
  try {
    fs.accessSync(path.join(dir, name), fs.constants.X_OK);
    return `./${name}`;
  } catch { return fallback; }
}

/**
 * Does this Gradle build actually compile JVM code? Unlike Maven, whose
 * lifecycle phases are fixed by the tool, Gradle tasks are defined by the build
 * script — `test` does not exist in an Android or a pure aggregator build, and
 * calling it produces `Task 'test' not found`: a non-zero exit with no file:line
 * anywhere, i.e. an empty signature that would compare equal to itself forever.
 * So we look for evidence that a JVM plugin is applied, and stay silent without
 * it. Reading a build script as text is the same move the Python block makes
 * with pyproject.toml, for the same reason: no parser, no dependency.
 */
const JVM_PLUGIN_RE = /\bid\s*[("']\s*(?:java|java-library|application|groovy|scala)\b|\bkotlin\s*\(\s*["']jvm|org\.jetbrains\.kotlin\.jvm|apply\s+plugin:\s*['"](?:java|java-library|application|groovy|scala|kotlin)['"]/;

/**
 * Build scripts of the area plus one level of subprojects: the common
 * multi-project layout leaves the root script empty and applies the plugin in
 * each subproject, and a root-only check would call that build unbuildable.
 */
function gradleScripts(dir, maxFiles = 50) {
  const names = ['build.gradle', 'build.gradle.kts', 'settings.gradle', 'settings.gradle.kts'];
  const read = (d) => names.map((n) => {
    try { return fs.readFileSync(path.join(d, n), 'utf8'); } catch { return ''; }
  }).join('\n');

  let text = read(dir);
  let seen = 0;
  let entries = [];
  try { entries = fs.readdirSync(dir, { withFileTypes: true }); } catch { /* unreadable */ }
  for (const e of entries) {
    if (seen >= maxFiles) break;
    if (!e.isDirectory() || PRUNE.has(e.name) || e.name.startsWith('.')) continue;
    text += '\n' + read(path.join(dir, e.name));
    seen += 1;
  }
  return text;
}

/** CI `run:` lines are recorded as hints and never executed — they assume services and secrets. */
export function ciHints(root) {
  const dir = path.join(root, '.github', 'workflows');
  const hints = [];
  let files = [];
  try { files = fs.readdirSync(dir).filter((f) => /\.ya?ml$/.test(f)); } catch { return hints; }
  for (const f of files) {
    let text = '';
    try { text = fs.readFileSync(path.join(dir, f), 'utf8'); } catch { continue; }
    for (const line of text.split('\n')) {
      const m = /^\s*(?:-\s*)?run:\s*(.+)$/.exec(line);
      if (m && /\b(test|lint|build|typecheck|check)\b/.test(m[1])) {
        hints.push({ file: `.github/workflows/${f}`, command: m[1].trim() });
        if (hints.length >= 20) return hints;
      }
    }
  }
  return hints;
}

/**
 * Full discovery. `.refactor/commands.json` with "locked": true short-circuits
 * everything, which lets this code stay dumb forever.
 */
export function discoverCommands(root, { lockedFile } = {}) {
  if (lockedFile && fs.existsSync(lockedFile)) {
    const locked = readJsonSafe(lockedFile);
    if (locked && locked.locked === true && Array.isArray(locked.commands)) {
      return { areas: [...new Set(locked.commands.map((c) => c.area ?? '.'))], commands: locked.commands, skipped: [], hints: [], locked: true };
    }
  }
  const areas = detectAreas(root);
  const all = areas.flatMap((a) => discoverArea(root, a));
  return {
    areas,
    commands: all.filter((c) => c.tier !== 'SKIPPED'),
    skipped: all.filter((c) => c.tier === 'SKIPPED'),
    hints: ciHints(root),
    locked: false,
  };
}

// ---------------------------------------------------------------- execution

/**
 * Run one command. No shell — argv is spawned directly, so `&&`, globs and
 * variable expansion are impossible by construction. SIGTERM, then SIGKILL to
 * the whole process group 10s later.
 */
export function runCommand(cmd, wt, { onLine } = {}) {
  return new Promise((resolve) => {
    const started = Date.now();
    const child = spawn(cmd.argv[0], cmd.argv.slice(1), {
      cwd: path.join(wt, cmd.cwd ?? '.'),
      env: { ...process.env, CI: '1', TZ: 'UTC', FORCE_COLOR: '0', NO_COLOR: '1' },
      detached: true,
      stdio: ['ignore', 'pipe', 'pipe'],
    });
    let stdout = '', stderr = '', timedOut = false, killer = null, spawnError = null;

    const cap = (buf, which) => {
      const s = buf.toString();
      if (which === 'out') stdout += s; else stderr += s;
      if (onLine) for (const l of s.split('\n')) if (l.trim()) onLine(l);
    };
    child.stdout.on('data', (b) => cap(b, 'out'));
    child.stderr.on('data', (b) => cap(b, 'err'));

    const timer = setTimeout(() => {
      timedOut = true;
      try { process.kill(-child.pid, 'SIGTERM'); } catch { /* already gone */ }
      killer = setTimeout(() => { try { process.kill(-child.pid, 'SIGKILL'); } catch { /* gone */ } }, 10_000);
    }, cmd.timeoutMs || TIER_TIMEOUT_MS.T2);

    const done = (code) => {
      clearTimeout(timer); if (killer) clearTimeout(killer);
      const durationMs = Date.now() - started;
      resolve({
        id: cmd.id, name: cmd.name, area: cmd.area, argv: cmd.argv, cwd: cmd.cwd, tier: cmd.tier,
        exitCode: timedOut ? null : code,
        timedOut, spawnError, durationMs,
        stdoutTail: tail(stdout), stderrTail: tail(stderr),
        signature: extractSignature(stdout + '\n' + stderr),
      });
    };
    // The process never started: no exit code of ours can honestly describe that.
    child.on('error', (e) => { spawnError = e.code ?? 'SPAWN_FAILED'; stderr += String(e.message); done(-1); });
    child.on('close', done);
  });
}

const tail = (s, n = 4096) => (s.length <= n ? s : s.slice(-n));

// ---------------------------------------------------------------- signatures

// A signature is made only of CODE-LEVEL complaints. Build-level status lines —
// "BUILD FAILURE", "Execution failed for task ':app:test'" — are deliberately
// absent: they are identical no matter what the code does, so a signature made
// of them is stable, meaningless, and passes the differential gate forever. That
// is the same failure classify() guards against for UNRUNNABLE, and it is worth
// re-reading before adding a pattern here.
const SIG_PATTERNS = [
  // file:line — the dominant shape for compilers and type checkers
  { re: /([\w./\-]+\.(?:ts|tsx|js|jsx|mjs|cjs|go|py|rs|java|rb|kt|kts|scala|groovy|swift)):(\d+)/g, fmt: (m) => `${m[1]}:${m[2]}` },
  // Maven's compiler puts the position in brackets, so the pattern above — which
  // wants a digit straight after the colon — never fires on it. Normalised to
  // the same `path:line` shape so the line-drift tolerance below applies.
  //   [ERROR] /w/src/main/java/Foo.java:[7,9] cannot find symbol
  { re: /([\w./\-]+\.(?:java|kt|kts|scala|groovy)):\[(\d+),\d+\]/g, fmt: (m) => `${m[1]}:${m[2]}` },
  // Kotlin 1.8 and older: `e: /w/src/Foo.kt: (12, 9): Unresolved reference: bar`
  { re: /([\w./\-]+\.kts?):\s*\((\d+),\s*\d+\)/g, fmt: (m) => `${m[1]}:${m[2]}` },
  // Gradle / JUnit:  `CalculatorTest > testAdd() FAILED`
  { re: /^\s*([\w.$]+(?:\s*>\s*[^\n>]+?)+)\s+FAILED\s*$/gm, fmt: (m) => `JUNIT:${m[1].replace(/\s+/g, ' ')}` },
  // Maven surefire, 3.x (`--`) and older (bare) alike. The elapsed time is
  // excluded on purpose: it changes on every run, and a signature that never
  // repeats is as useless as one that never changes.
  //   [ERROR] com.x.CalculatorTest.testAdd -- Time elapsed: 0.008 s <<< FAILURE!
  //   [ERROR] testAdd(com.x.CalcTest)  Time elapsed: 1.204 s  <<< ERROR!
  { re: /([\w.$]+(?:\([\w.$]+\))?)\s+(?:--\s+)?Time elapsed:[^\n]*<<<\s*(?:FAILURE|ERROR)!/g, fmt: (m) => `SUREFIRE:${m[1]}` },
  // error codes
  { re: /\b(TS\d{4}|E\d{4}|SA\d{4}|L\d{4}|B\d{4})\b/g, fmt: (m) => m[1] },
  // go test
  { re: /---\s+FAIL:\s+([A-Za-z0-9_/]+)/g, fmt: (m) => `GOFAIL:${m[1]}` },
  // TAP
  { re: /^not ok \d+ [-–] (.+)$/gm, fmt: (m) => `TAP:${m[1].trim()}` },
  // node:test / vitest default reporters
  { re: /^\s*[✖✗×]\s+(.+?)(?:\s+\(\d|$)/gm, fmt: (m) => `FAIL:${m[1].trim()}` },
];

/** Deduped, sorted, order-independent fingerprint of a command's complaints. */
export function extractSignature(text) {
  const out = new Set();
  for (const { re, fmt } of SIG_PATTERNS) {
    re.lastIndex = 0;
    let m;
    while ((m = re.exec(text)) !== null) out.add(fmt(m));
  }
  return [...out].sort();
}

const fileOf = (sig) => {
  const m = /^(.+):(\d+)$/.exec(sig);
  return m ? m[1] : null;
};

/**
 * Behavior preservation is a DIFFERENTIAL property. For a command that was
 * already RED, "it still fails" is the expected outcome; what matters is that
 * it fails in exactly the same places.
 *
 * Line numbers shift whenever code above them moves, so a `file:line` entry that
 * is new is accepted when that FILE already complained at baseline, and rejected
 * when the file is new to the signature.
 */
export function signatureDelta(before, after) {
  const beforeSet = new Set(before);
  const beforeFiles = new Set(before.map(fileOf).filter(Boolean));
  const added = [];
  const drifted = [];
  for (const s of after) {
    if (beforeSet.has(s)) continue;
    const f = fileOf(s);
    if (f && beforeFiles.has(f)) { drifted.push(s); continue; }   // same file, moved line
    added.push(s);
  }
  return { ok: added.length === 0, added, drifted, removed: before.filter((s) => !after.includes(s)) };
}

// ---------------------------------------------------------------- baseline

export const STATUS = { GREEN: 'GREEN', RED: 'RED', TIMEOUT: 'TIMEOUT', UNRUNNABLE: 'UNRUNNABLE', OPAQUE: 'OPAQUE' };

/**
 * "The binary is not installed" and "the check fails" are different facts and
 * must never share a status. A command that could not be spawned has an EMPTY
 * signature, so folding it into RED would mint a differential gate that passes
 * unconditionally — a check that validates nothing while reporting that it did.
 *
 * OPAQUE is the same hazard arriving by a different road: the command RAN, and
 * failed, but said nothing this file knows how to read. That happens for real —
 * a Gradle build that cannot resolve dependencies or download a JDK toolchain
 * exits non-zero with no file:line anywhere — and an empty signature compares
 * equal to itself forever. Excluded from evidence for exactly the same reason,
 * and named differently because the fix is different: UNRUNNABLE means install
 * the tool, OPAQUE means the tool ran and we cannot grade it.
 */
export function classify(result) {
  if (result.spawnError) return STATUS.UNRUNNABLE;
  if (result.timedOut) return STATUS.TIMEOUT;
  if (result.exitCode === 0) return STATUS.GREEN;
  return (result.signature?.length ?? 0) === 0 ? STATUS.OPAQUE : STATUS.RED;
}

/**
 * Run every discovered command once. A RED command is not fatal — it is demoted
 * to a differential check. Only a baseline with ZERO green commands aborts the
 * run: with no trustworthy signal there is no behavior-preservation evidence,
 * and proceeding would be theatre.
 */
export async function runBaseline(commands, wt, log) {
  const results = [];
  for (const cmd of commands) {
    log?.info(`${cmd.area} ${cmd.tier}  ${cmd.argv.join(' ')}`, { cwd: cmd.cwd });
    const r = await runCommand(cmd, wt);
    r.status = classify(r);
    results.push(r);
    const secs = Math.round(r.durationMs / 1000);
    if (r.status === STATUS.GREEN) log?.pass(`${cmd.area} ${cmd.tier} ${cmd.name}  exit 0 · ${secs}s → GREEN`);
    else if (r.status === STATUS.UNRUNNABLE) log?.warn(`${cmd.area} ${cmd.tier} ${cmd.name}  ${cmd.argv[0]}: ${r.spawnError} → excluded (never ran, so it is not evidence)`);
    else if (r.status === STATUS.TIMEOUT) log?.warn(`${cmd.area} ${cmd.tier} ${cmd.name}  timed out after ${secs}s → excluded`);
    else if (r.status === STATUS.OPAQUE) log?.warn(`${cmd.area} ${cmd.tier} ${cmd.name}  exit ${r.exitCode} · ${secs}s → excluded (failed with no recognisable output, so it cannot be a differential check)`);
    else log?.fail(`${cmd.area} ${cmd.tier} ${cmd.name}  exit ${r.exitCode} · ${secs}s → RED (${r.signature.length} sig)`);
  }
  const green = results.filter((r) => r.status === STATUS.GREEN);
  const red = results.filter((r) => r.status === STATUS.RED);
  const unrunnable = results.filter((r) => r.status === STATUS.UNRUNNABLE);
  const opaque = results.filter((r) => r.status === STATUS.OPAQUE);
  return {
    results,
    green: green.length,
    red: red.length,
    unrunnable,
    opaque,
    usable: green.length > 0,
    describe: `GREEN ${green.length} / RED-differential ${red.length} of ${results.length}`
      + (unrunnable.length ? ` · ${unrunnable.length} excluded (not executable)` : '')
      + (opaque.length ? ` · ${opaque.length} excluded (no readable output)` : ''),
  };
}

/** Areas whose subtree contains at least one changed path, plus the root area. */
export function areasForPaths(areas, changedPaths) {
  const hit = new Set();
  for (const p of changedPaths) {
    let best = '.';
    for (const a of areas) {
      if (a === '.') continue;
      if (p === a || p.startsWith(`${a}/`)) { if (a.length > best.length || best === '.') best = a; }
    }
    hit.add(best);
  }
  if (areas.includes('.')) hit.add('.');
  return [...hit];
}

/**
 * Post-candidate validation. Scoped to the areas actually touched, compared
 * against the baseline rather than against zero.
 */
export async function runLadder(baseline, commands, wt, changed, areas, log) {
  const scope = new Set(areasForPaths(areas, changed));
  const byId = new Map(baseline.results.map((r) => [r.id, r]));
  const checks = [];
  for (const cmd of commands) {
    if (!scope.has(cmd.area)) continue;
    const base = byId.get(cmd.id);
    if (base && base.status === STATUS.TIMEOUT) continue;             // never fenced the baseline either
    if (base && base.status === STATUS.UNRUNNABLE) continue;           // produced no evidence to compare against
    if (base && base.status === STATUS.OPAQUE) continue;               // failed unreadably; comparing it to itself always passes

    log?.info(`${cmd.area} ${cmd.tier}  ${cmd.argv.join(' ')}`, { cwd: cmd.cwd });
    const r = await runCommand(cmd, wt);
    r.status = classify(r);
    const secs = Math.round(r.durationMs / 1000);

    let ok, why;
    if (r.spawnError) {
      // Not a regression — an environment that stopped being able to answer.
      // Rolling the candidate back is still the safe move: we cannot clear it.
      ok = false; why = `${cmd.argv[0]}: ${r.spawnError} — the command could not be executed`;
      r.failureKind = 'VALIDATION_UNRUNNABLE';
    } else if (r.timedOut) {
      ok = false; why = `timed out after ${secs}s`;
      r.failureKind = 'VALIDATION_TIMEOUT';
    } else if (!base || base.status === STATUS.GREEN) {
      ok = r.exitCode === 0;
      why = ok ? `exit 0 · ${secs}s` : `exit ${r.exitCode} · ${secs}s (was GREEN at baseline)`;
      if (!ok) r.failureKind = 'REGRESSION';
    } else if (r.status === STATUS.OPAQUE) {
      // It failed before and it fails now, but it stopped saying anything we can
      // read — so there is no longer a signature to compare. signatureDelta
      // would report "no new signature" and wave it through, because nothing was
      // added to an empty set. That is the same unconditional pass the baseline
      // excludes OPAQUE to avoid; the ladder has to refuse it too.
      ok = false;
      why = `exit ${r.exitCode} · ${secs}s · failed with no recognisable output, so it cannot be compared to its baseline (${base.signature.length} signature(s))`;
      r.failureKind = 'REGRESSION';
    } else {
      const d = signatureDelta(base.signature, r.signature);
      ok = d.ok;
      why = ok
        ? `exit ${r.exitCode} · ${secs}s · RED-differential, no new signature (${r.signature.length}/${base.signature.length}${d.drifted.length ? `, ${d.drifted.length} line-drift` : ''})`
        : `exit ${r.exitCode} · ${secs}s · ${d.added.length} NEW signature: ${d.added.slice(0, 3).join(', ')}`;
      r.delta = d;
      if (!ok) r.failureKind = 'REGRESSION';
    }
    r.ok = ok; r.why = why;
    checks.push(r);
    if (ok) log?.pass(`${cmd.area} ${cmd.tier} ${cmd.name}  ${why}`);
    else log?.fail(`${cmd.area} ${cmd.tier} ${cmd.name}  ${why}`);
    if (!ok) break;                                                    // first failure ends the ladder
  }
  const failed = checks.find((c) => !c.ok) ?? null;
  return { ok: failed === null, checks, failed, scope: [...scope] };
}
