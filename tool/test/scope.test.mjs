import { test } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { normalizeTargets, isUnderTarget, touchesTarget, sourceFiles } from '../src/scope.mjs';

function repo() {
  const root = fs.realpathSync(fs.mkdtempSync(path.join(os.tmpdir(), 'rl-scope-')));
  fs.mkdirSync(path.join(root, 'app', 'web', 'src'), { recursive: true });
  fs.mkdirSync(path.join(root, 'app', 'website'), { recursive: true });
  fs.writeFileSync(path.join(root, 'README.md'), '#\n');
  return root;
}

// The bug this exists to prevent is silent: `startsWith('app/web')` matches
// `app/website`, and a run scoped to one app would happily audit the other.
test('containment respects the path separator', () => {
  assert.ok(isUnderTarget('app/web/src/a.ts', ['app/web']));
  assert.ok(isUnderTarget('app/web', ['app/web']), 'the directory itself is in scope');
  assert.ok(!isUnderTarget('app/website/a.ts', ['app/web']));
  assert.ok(!isUnderTarget('src/a.ts', ['app/web']));
});

// No target means no restriction. If this ever returned false for an empty
// target list, rank() would filter out every candidate and the loop would
// silently do nothing on an ordinary unscoped run.
test('an empty target list restricts nothing', () => {
  assert.ok(isUnderTarget('anything/at/all.ts', []));
  assert.ok(touchesTarget([], []));
  assert.ok(touchesTarget(['x.ts'], undefined));
});

// The whole point of the feature: one foot inside is enough, because the
// callers a candidate must also edit are legitimately outside.
test('a candidate needs only one related file inside the scope', () => {
  const t = ['app/web'];
  assert.ok(touchesTarget(['app/web/a.ts', 'src/caller.ts', 'src/b.test.ts'], t));
  assert.ok(!touchesTarget(['src/caller.ts', 'src/b.test.ts'], t));
  assert.ok(!touchesTarget([], t));
});

test('targets resolve against the directory the operator typed them in', () => {
  const root = repo();
  const { targets, errors } = normalizeTargets(root, ['src'], path.join(root, 'app', 'web'));
  assert.deepEqual(errors, []);
  assert.deepEqual(targets, ['app/web/src'], 'cwd-relative, not repo-root-relative');
});

test('a trailing slash and a duplicate collapse to one target', () => {
  const root = repo();
  const { targets } = normalizeTargets(root, ['app/web/', 'app/web'], root);
  assert.deepEqual(targets, ['app/web']);
});

// Both would produce identical containment answers, so keeping both would only
// make the log claim two areas are in scope when one is.
test('a nested target collapses into its parent', () => {
  const root = repo();
  const { targets } = normalizeTargets(root, ['app/web/src', 'app/web'], root);
  assert.deepEqual(targets, ['app/web']);
});

test('a target outside the repository is refused', () => {
  const root = repo();
  const { targets, errors } = normalizeTargets(root, ['..'], root);
  assert.deepEqual(targets, []);
  assert.match(errors[0], /outside the repository/);
});

test('a missing directory and a file are both refused', () => {
  const root = repo();
  const missing = normalizeTargets(root, ['nope'], root);
  assert.match(missing.errors[0], /does not exist/);
  const file = normalizeTargets(root, ['README.md'], root);
  assert.match(file.errors[0], /not a directory/);
});

// Accepting it would be worse than refusing: the run would look scoped in the
// banner and the report while behaving exactly like an unscoped one.
test('the repository root is refused rather than treated as a scope', () => {
  const root = repo();
  const { targets, errors } = normalizeTargets(root, ['.'], root);
  assert.deepEqual(targets, []);
  assert.match(errors[0], /repository root/);
});

// doctor and the audit inventory must agree on whether a target is empty; they
// agree by calling this.
test('source detection skips vendored trees', () => {
  const out = sourceFiles([
    'app/web/a.ts', 'app/web/node_modules/dep/index.js', 'vendor/x/y.go',
    'app/web/dist/bundle.js', 'README.md', 'app/api/main.go',
  ]);
  assert.deepEqual(out, ['app/web/a.ts', 'app/api/main.go']);
});
