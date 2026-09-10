import { test } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { SCHEMAS, validate, enforceExecutionVerdict, enforcePreflightVerdict,
  enforceCharacterizationVerdict, enforceReviewVerdict } from '../src/schemas.mjs';
import { rank, DEFAULT_POLICY } from '../src/loop.mjs';
import { newState } from '../src/state.mjs';
import { runBaseline, runLadder, areasForPaths } from '../src/validate.mjs';

function examples(file) {
  const text = fs.readFileSync(new URL(file, import.meta.url), 'utf8');
  return new Map([...text.matchAll(/<!-- example: ([\w-]+) schema: (\w+) -->\n```json\n([\s\S]*?)\n```/g)]
    .map(([, name, schema, json]) => [name, { schema, value: JSON.parse(json) }]));
}
const english = examples('../WORKFLOW.md');
const korean = examples('../WORKFLOW.ko.md');
const value = (name) => structuredClone(english.get(name).value);

test('Workflow editions share full JSON examples that match the actual phase schemas', () => {
  assert.equal(english.size, 10);
  assert.deepEqual(korean, english);
  for (const [name, { schema, value: example }] of english) {
    assert.ok(SCHEMAS[schema], `${name}: unknown schema`);
    assert.deepEqual(validate(SCHEMAS[schema], example), [], name);
  }
});

test('Workflow success and rejection responses follow the current verdict enforcers', () => {
  const packet = value('deepcheck-success');
  assert.equal(enforceExecutionVerdict(value('execute-success'), packet).verdict, 'PASS');
  assert.equal(enforcePreflightVerdict(value('preflight-success')).verdict, 'READY_TO_EXECUTE');
  assert.equal(enforceReviewVerdict(value('review-success')).verdict, 'PASS');
  assert.equal(enforcePreflightVerdict(value('preflight-blocked')).verdict, 'BLOCKED');
  assert.equal(enforceReviewVerdict(value('review-rejected')).verdict, 'FAIL');
  assert.equal(enforceCharacterizationVerdict(value('characterization-success')).verdict, 'PASS');
  const characterizationPacket = value('characterization-packet');
  assert.equal(characterizationPacket.characterization_needed, true);
  assert.deepEqual(characterizationPacket.characterization_files, value('characterization-success').changed_files);
  for (const falsification_result of ['SURVIVED', 'INCONCLUSIVE']) {
    assert.equal(enforcePreflightVerdict({ ...value('preflight-success'), falsification_result }).verdict, 'BLOCKED');
  }
  for (const classification of ['UNEXPLAINED', 'CONTRACT_CHANGING']) {
    const execution = value('execute-success'); execution.hunks[0].classification = classification;
    assert.equal(enforceExecutionVerdict(execution, packet).verdict, 'FAIL');
  }
  for (const flag of ['production_source_changed', 'assertions_weakened']) {
    assert.equal(enforceCharacterizationVerdict({ ...value('characterization-success'), [flag]: true }).verdict, 'FAIL');
  }
  const high = value('review-rejected'); high.verdict = 'PASS'; high.behavior_preservation_assessment = 'PRESERVED'; high.findings[0].severity = 'HIGH';
  assert.equal(enforceReviewVerdict(high).verdict, 'PASS');
});

test('audit UNKNOWN is filtered while NEEDS_EVIDENCE can reach deep check', () => {
  const state = newState({ id: 'workflow', repoRoot: '/fixture', providers: {} });
  const ctx = { state, policy: DEFAULT_POLICY, log: { info() {} }, store: { save() {} }, targets: [] };
  const candidate = value('audit-success').candidates[0];
  assert.equal(rank(ctx, [{ ...candidate, risk_level: 'UNKNOWN' }]).length, 0);
  assert.equal(state.seen.skipped.at(-1).reason, 'RISK_UNKNOWN');
  state.seen.skipped = [];
  assert.equal(rank(ctx, [{ ...candidate, readiness: 'NEEDS_EVIDENCE' }]).length, 1);
  assert.equal(value('deepcheck-evidence').readiness, 'NEEDS_EVIDENCE');
});

test('validation follows the documented affected-area scope and rejects a new regression', async (t) => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'workflow-validation-'));
  t.after(() => fs.rmSync(dir, { recursive: true, force: true }));
  for (const area of ['app', 'other']) fs.mkdirSync(path.join(dir, area));
  fs.writeFileSync(path.join(dir, 'app/check.mjs'), 'process.exit(0);\n');
  const commands = [
    { id: 'root', area: '.', tier: 'T1', name: 'root', argv: [process.execPath, '-e', 'process.exit(0)'], cwd: '.', timeoutMs: 1000 },
    { id: 'app', area: 'app', tier: 'T2', name: 'app', argv: [process.execPath, 'check.mjs'], cwd: 'app', timeoutMs: 1000 },
    { id: 'other', area: 'other', tier: 'T2', name: 'other', argv: [process.execPath, '-e', 'process.exit(0)'], cwd: 'other', timeoutMs: 1000 },
  ];
  const baseline = await runBaseline(commands, dir);
  assert.equal(baseline.results.length, 3); assert.equal(baseline.usable, true);
  assert.deepEqual(areasForPaths(['.', 'app', 'app/nested', 'other'], ['app/nested/source.mjs']), ['app/nested', '.']);
  const pass = await runLadder(baseline, commands, dir, ['app/source.mjs'], ['.', 'app', 'other']);
  assert.equal(pass.ok, true);
  assert.deepEqual(pass.checks.map((r) => r.id), ['root', 'app']);
  fs.writeFileSync(path.join(dir, 'app/check.mjs'), 'process.exit(1);\n');
  const fail = await runLadder(baseline, commands, dir, ['app/source.mjs'], ['.', 'app', 'other']);
  assert.equal(fail.ok, false); assert.equal(fail.checks.at(-1).failureKind, 'REGRESSION');
});
