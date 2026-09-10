// Java/Kotlin support. Everything here is toolchain-free: discovery reads the
// filesystem and never consults PATH, so these pass on a machine with no JDK,
// no gradle and no maven.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { discoverArea, detectAreas, extractSignature, classify, STATUS, runCommand, signatureDelta } from '../src/validate.mjs';
import { isTestPath, WEAKENING_RE, FORBIDDEN_GLOBS, globToRegex, matchesAny } from '../src/gate.mjs';

const tmp = (tag) => fs.mkdtempSync(path.join(os.tmpdir(), `rl-${tag}-`));
const write = (dir, rel, body) => {
  fs.mkdirSync(path.join(dir, path.dirname(rel)), { recursive: true });
  fs.writeFileSync(path.join(dir, rel), body);
};
const argvs = (cmds) => cmds.map((c) => c.argv.join(' '));
const forbidden = (p) => matchesAny(p, FORBIDDEN_GLOBS.map(globToRegex));

// ------------------------------------------------------------- discovery

test('a gradle build with a JVM plugin gets a compile check and a test run', () => {
  const d = tmp('gradle');
  try {
    write(d, 'build.gradle', "plugins { id 'java' }");
    const cmds = discoverArea(d, '.');
    assert.deepEqual(argvs(cmds), ['gradle testClasses --console=plain', 'gradle test --console=plain']);
    assert.deepEqual(cmds.map((c) => c.tier), ['T1', 'T2']);
    // No T3: `assemble` would recompile what T2 already compiled, once per
    // candidate, and it is the most expensive rung in the ladder.
    assert.equal(cmds.filter((c) => c.tier === 'T3').length, 0);
  } finally { fs.rmSync(d, { recursive: true, force: true }); }
});

test('a Kotlin JVM build in kts is recognised too', () => {
  const d = tmp('kts');
  try {
    write(d, 'settings.gradle.kts', 'rootProject.name = "x"');
    write(d, 'build.gradle.kts', 'plugins { kotlin("jvm") version "2.0.0" }');
    assert.equal(discoverArea(d, '.').length, 2);
  } finally { fs.rmSync(d, { recursive: true, force: true }); }
});

test('a gradle build with no JVM plugin contributes nothing at all', () => {
  // Gradle tasks are defined by the build script, not by the tool. Calling
  // `test` on an Android or aggregator build gives "Task 'test' not found":
  // a non-zero exit with an empty signature, which is a gate that passes
  // unconditionally. Silence is the honest answer; commands.json is the escape.
  const d = tmp('android');
  try {
    write(d, 'build.gradle', "plugins { id 'com.android.application' }");
    assert.deepEqual(discoverArea(d, '.'), []);
  } finally { fs.rmSync(d, { recursive: true, force: true }); }
});

test('a plugin applied only in a subproject still counts', () => {
  // The common multi-project layout leaves the root script empty.
  const d = tmp('multi');
  try {
    write(d, 'settings.gradle', "include 'svc'");
    write(d, 'svc/build.gradle', "plugins { id 'java' }");
    assert.equal(discoverArea(d, '.').length, 2);
  } finally { fs.rmSync(d, { recursive: true, force: true }); }
});

test('the gradle wrapper is used only when it is actually executable', () => {
  // A wrapper committed from Windows arrives mode 644; spawning it fails with
  // EACCES and would take the whole area out as UNRUNNABLE, when the PATH
  // binary would have worked.
  const d = tmp('wrap');
  try {
    write(d, 'build.gradle', "plugins { id 'java' }");
    write(d, 'gradlew', '#!/bin/sh\nexit 0\n');
    fs.chmodSync(path.join(d, 'gradlew'), 0o755);
    assert.equal(discoverArea(d, '.')[0].argv[0], './gradlew');
    fs.chmodSync(path.join(d, 'gradlew'), 0o644);
    assert.equal(discoverArea(d, '.')[0].argv[0], 'gradle');
  } finally { fs.rmSync(d, { recursive: true, force: true }); }
});

test('maven needs no plugin evidence, because its phases are fixed by the tool', () => {
  const d = tmp('mvn');
  try {
    write(d, 'pom.xml', '<project><artifactId>x</artifactId></project>');
    assert.deepEqual(argvs(discoverArea(d, '.')), ['mvn -B test-compile', 'mvn -B test']);
    write(d, 'mvnw', '#!/bin/sh\nexit 0\n');
    fs.chmodSync(path.join(d, 'mvnw'), 0o755);
    assert.equal(discoverArea(d, '.')[0].argv[0], './mvnw');
  } finally { fs.rmSync(d, { recursive: true, force: true }); }
});

test('a nested JVM module is not an area of its own', () => {
  // If it were, areasForPaths would map a changed file to the module, the area
  // that actually owns the commands would drop out of scope, and the JVM checks
  // would silently not run while the report claimed validation passed.
  const d = tmp('nest');
  try {
    write(d, 'pom.xml', '<project/>');
    write(d, 'app/pom.xml', '<project/>');
    assert.deepEqual(detectAreas(d), ['.']);
    assert.deepEqual(discoverArea(d, 'app'), []);
  } finally { fs.rmSync(d, { recursive: true, force: true }); }
});

test('a JS front end beside a gradle build keeps both test commands', () => {
  // Regression: ids were `area:tier:name`, so two `test` commands collided and
  // runLadder's Map kept only the last — comparing one command's run against
  // the OTHER command's baseline.
  const d = tmp('mixed');
  try {
    write(d, 'package.json', JSON.stringify({ scripts: { test: 'node --test' } }));
    write(d, 'build.gradle', "plugins { id 'java' }");
    const cmds = discoverArea(d, '.');
    assert.equal(new Set(cmds.map((c) => c.id)).size, cmds.length);
    assert.equal(cmds.filter((c) => c.name === 'test').length, 2);
  } finally { fs.rmSync(d, { recursive: true, force: true }); }
});

test('a relative wrapper resolves against the area, not the process cwd', () => {
  // The single execution assumption the wrapper strategy rests on: spawn chdirs
  // to cwd before exec. An absolute path is not an option — it would bake the
  // discovery root into a spec that outlives its worktree.
  const wt = tmp('rel');
  try {
    write(wt, 'svc/gradlew', '#!/bin/sh\necho ok\n');
    fs.chmodSync(path.join(wt, 'svc', 'gradlew'), 0o755);
    return runCommand({ id: 'x', name: 'test', area: 'svc', tier: 'T2', argv: ['./gradlew'], cwd: 'svc', timeoutMs: 10_000 }, wt)
      .then((r) => {
        assert.equal(r.spawnError, null);
        assert.equal(r.exitCode, 0);
      })
      .finally(() => fs.rmSync(wt, { recursive: true, force: true }));
  } catch (e) { fs.rmSync(wt, { recursive: true, force: true }); throw e; }
});

// ------------------------------------------------------------- signatures

test('JVM failures produce a signature at all', () => {
  // Without these the differential gate is not merely weaker, it is inert: an
  // empty signature compares equal to itself forever, so every candidate passes
  // while the report says the checks ran.
  const cases = [
    ['[ERROR] /w/src/main/java/com/x/Foo.java:[7,9] cannot find symbol', '/w/src/main/java/com/x/Foo.java:7'],
    ['/w/src/main/java/com/x/Foo.java:7: error: cannot find symbol', '/w/src/main/java/com/x/Foo.java:7'],
    ['e: /w/src/Foo.kt: (12, 9): Unresolved reference: bar', '/w/src/Foo.kt:12'],
    ['CalculatorTest > testAdd() FAILED', 'JUNIT:CalculatorTest > testAdd()'],
    ['[ERROR] com.x.CalcTest.testAdd -- Time elapsed: 0.008 s <<< FAILURE!', 'SUREFIRE:com.x.CalcTest.testAdd'],
    ['[ERROR] testAdd(com.x.CalcTest)  Time elapsed: 1.204 s  <<< ERROR!', 'SUREFIRE:testAdd(com.x.CalcTest)'],
  ];
  for (const [text, want] of cases) assert.ok(extractSignature(text).includes(want), text);
});

test('build-level status lines are deliberately not signatures', () => {
  // They are identical no matter what the code does. A signature made of them
  // is stable, meaningless, and launders an infrastructure failure into a
  // passing differential check.
  assert.deepEqual(extractSignature([
    'FAILURE: Build failed with an exception.',
    "Execution failed for task ':app:compileJava'.",
    '> Task :app:test FAILED',
    '[ERROR] BUILD FAILURE',
  ].join('\n')), []);
});

test('a test count is not a signature, because adding a test would trip it', () => {
  // The gate permits adding tests; "Tests run: 5" → "Tests run: 6" must not
  // read as a regression.
  assert.deepEqual(extractSignature('Tests run: 5, Failures: 1, Errors: 0, Skipped: 0'), []);
});

test('surefire elapsed time never enters the signature', () => {
  const a = extractSignature('[ERROR] com.x.T.a -- Time elapsed: 0.008 s <<< FAILURE!');
  const b = extractSignature('[ERROR] com.x.T.a -- Time elapsed: 9.911 s <<< FAILURE!');
  assert.deepEqual(a, b);
});

test('a Maven column change is tolerated the way a line shift is', () => {
  const before = extractSignature('[ERROR] /w/Foo.java:[7,9] cannot find symbol');
  const after = extractSignature('[ERROR] /w/Foo.java:[9,4] cannot find symbol');
  const d = signatureDelta(before, after);
  assert.equal(d.ok, true);
  assert.equal(d.drifted.length, 1);
});

// ------------------------------------------------------------- OPAQUE

test('a failure nobody can read is excluded rather than trusted', () => {
  // A Gradle build that cannot resolve dependencies or fetch a JDK toolchain
  // exits non-zero with no file:line anywhere. Treated as RED it would be a
  // differential check that passes unconditionally.
  assert.equal(classify({ exitCode: 1, signature: [] }), STATUS.OPAQUE);
  assert.equal(classify({ exitCode: 1, signature: ['Foo.java:7'] }), STATUS.RED);
  assert.equal(classify({ exitCode: 0, signature: [] }), STATUS.GREEN);
  // the pre-existing exclusions keep their own names, because the fixes differ
  assert.equal(classify({ spawnError: 'ENOENT', exitCode: -1, signature: [] }), STATUS.UNRUNNABLE);
  assert.equal(classify({ timedOut: true, exitCode: null, signature: [] }), STATUS.TIMEOUT);
});

// ------------------------------------------------------------- gate

test('JVM manifests cannot be edited by the agent', () => {
  // loop.mjs skips dependency snapshotting on the grounds that every manifest is
  // hard-forbidden. That premise was false for Java.
  for (const p of [
    'build.gradle', 'app/build.gradle.kts', 'versions.gradle', 'build-logic/x.gradle.kts',
    'gradle/libs.versions.toml', 'gradle.lockfile', 'gradle.properties',
    'gradle/wrapper/gradle-wrapper.properties', 'gradlew', 'svc/pom.xml', 'mvnw', '.mvn/jvm.config',
  ]) assert.ok(forbidden(p), p);
});

test('ordinary Java sources are still editable', () => {
  for (const p of ['src/main/java/Service.java', 'src/main/java/Gradleish.java', 'src/main/kotlin/A.kt']) {
    assert.ok(!forbidden(p), p);
  }
});

test('JVM golden resources are protected like every other oracle', () => {
  assert.ok(forbidden('src/test/resources/expected.json'));
});

test('JVM test paths are recognised as tests', () => {
  for (const p of [
    'src/test/java/x/FooTest.java', 'app/src/integrationTest/java/A.java',
    'x/CalcSpec.kt', 'y/OrderIT.java', 'a/UserTests.java',
  ]) assert.ok(isTestPath(p), p);
  for (const p of ['src/main/java/Latest.java', 'src/main/java/Contest.java']) {
    assert.ok(!isTestPath(p), p);
  }
});

test('disabling a JUnit test counts as weakening it', () => {
  for (const t of ['@Disabled', '@Ignore("flaky")', 'Assumptions.assumeTrue(x)', '@Test(enabled = false)']) {
    assert.ok(WEAKENING_RE.test(t), t);
  }
  // an import is not a disablement
  assert.ok(!WEAKENING_RE.test('import org.junit.jupiter.api.Disabled;'));
  assert.ok(!WEAKENING_RE.test('boolean enabled = true;'));
});

// ------------------------------------------------- regressions found in review

test('a build root at the repository root is not outranked by anything outside it', () => {
  // The ancestor walk started at dirname(root), which is outside the repository,
  // and never met its stop condition — so it climbed to the filesystem root. A
  // stray pom.xml in a parent directory (a CI workspace, a home directory)
  // silenced the whole project's validation with no message at all. This is the
  // commonest layout there is: the repository IS the build.
  const d = tmp('anc');
  try {
    write(d, 'pom.xml', '<project/>');                       // the "outside" marker
    write(d, 'repo/build.gradle', "plugins { id 'java' }");
    assert.equal(discoverArea(path.join(d, 'repo'), '.').length, 2);
  } finally { fs.rmSync(d, { recursive: true, force: true }); }
});

test('a check that stops being readable fails the ladder instead of passing it', () => {
  // signatureDelta compares what was ADDED, so an empty `after` yields "no new
  // signature" and waves the candidate through. That is the exact unconditional
  // pass OPAQUE exists to prevent, and excluding it at baseline was only half
  // the job — a check can also go unreadable between baseline and candidate.
  const d = signatureDelta(['Foo.java:7', 'JUNIT:CalcTest > add()'], []);
  assert.equal(d.ok, true, 'signatureDelta alone cannot see this');
  assert.equal(classify({ exitCode: 1, signature: [] }), STATUS.OPAQUE,
    'so runLadder must branch on the status, which is what it now does');
});

test('a legacy apply-plugin line names the plugin exactly', () => {
  // `apply plugin: 'kotlin-android'` starts with kotlin but is not a JVM build:
  // its test tasks are not the ones we would be calling.
  const d = tmp('legacy');
  try {
    write(d, 'build.gradle', "apply plugin: 'kotlin-android'");
    assert.deepEqual(discoverArea(d, '.'), []);
    write(d, 'build.gradle', "apply plugin: 'java'");
    assert.equal(discoverArea(d, '.').length, 2);
  } finally { fs.rmSync(d, { recursive: true, force: true }); }
});
