// schemas.mjs — the five contracts between the orchestrator and whichever model
// is running, plus a ~60-line validator.
//
// STRICTNESS POLICY: every schema here is written strict — additionalProperties
// false, every property required, no $ref/oneOf/anyOf/allOf/format. Optionality
// is expressed as a nullable type or an UNKNOWN enum member, never by omission.
// codex's --output-schema may require strict mode and claude's structured-output
// path is strict-flavoured; rather than probe and branch, we write the form that
// is valid input to BOTH, since a strict schema also satisfies a lenient
// validator and the reverse is not true.
//
// maxItems is documentation only. Structured-output implementations commonly
// ignore numeric array constraints, so the candidate cap is stated in the prompt
// AND enforced by truncation in the orchestrator.

const str = { type: 'string' };
const nstr = { type: ['string', 'null'] };
const int = { type: 'integer' };
const bool = { type: 'boolean' };
const arr = (items, maxItems) => (maxItems ? { type: 'array', items, maxItems } : { type: 'array', items });
const en = (...values) => ({ type: 'string', enum: values });

/** Build a strict object: additionalProperties false, everything required. */
function obj(properties) {
  return {
    type: 'object',
    additionalProperties: false,
    required: Object.keys(properties),
    properties,
  };
}

export const CATEGORIES = ['DEAD_CODE', 'COMPATIBILITY_REMOVAL', 'DEDUPLICATION', 'LARGE_COMPONENT_SPLIT'];
// UNKNOWN is a real answer, not a gap in the enum. sharpen-assess reports
// `change_risk: unknown` when the evidence cannot support a level, and a schema
// that cannot say so forces the model to pick a neighbouring value — which in
// practice means L0_LOW, the one level that gets executed. `rank` sets UNKNOWN
// candidates aside instead; see loop.mjs.
export const RISKS = ['L0_LOW', 'L1_MODERATE', 'L2_HIGH', 'L3_CRITICAL', 'UNKNOWN'];

// ---------------------------------------------------------------- candidate list

/**
 * `locator` is a symbol name or an exact quoted snippet — NEVER a line number.
 * Line numbers drift the moment anything above them changes and cannot be
 * re-verified by the other provider after a handoff; a quoted snippet is
 * grep-checkable by the orchestrator, which makes the evidence falsifiable
 * rather than decorative.
 *
 * GREP_ABSENCE is a first-class evidence kind because a dead-code claim is a
 * NEGATIVE claim: naming the absence you checked is what makes an audit auditable.
 */
const evidence = obj({
  file: str,
  locator: str,
  kind: en('GREP_HIT', 'GREP_ABSENCE', 'DEFINITION', 'EXPORT', 'TEST', 'CONFIG', 'GIT_HISTORY'),
  supports: str,
});

const candidate = obj({
  candidate_id: str,
  category: en(...CATEGORIES),
  title: str,
  related_files: arr(str),
  problem: str,
  minimal_change: str,
  observable_contracts: arr(str),
  static_reachability: en('NONE', 'INTERNAL_ONLY', 'EXPORTED_UNUSED', 'EXPORTED_USED', 'UNKNOWN'),
  dynamic_reachability: en('NONE', 'REFLECTION_OR_DI', 'STRING_KEYED', 'ROUTE_OR_CONFIG', 'UNKNOWN'),
  external_consumer_risk: en('NONE', 'INTERNAL_PACKAGE', 'PUBLIC_API', 'UNKNOWN'),
  ui_impact: en('NONE', 'VISUAL_ONLY', 'BEHAVIORAL', 'UNKNOWN'),
  persistence_impact: en('NONE', 'READ_PATH', 'WRITE_PATH', 'SCHEMA', 'UNKNOWN'),
  async_side_effect_impact: en('NONE', 'ORDERING', 'TIMING', 'CONCURRENCY', 'UNKNOWN'),
  test_protection: en('NONE', 'PARTIAL', 'STRONG', 'UNKNOWN'),
  characterization_needed: bool,
  estimated_file_count: int,
  primary_symbol: nstr,
  risk_level: en(...RISKS),
  readiness: en('READY', 'NEEDS_EVIDENCE', 'REJECT'),
  evidence: arr(evidence),
});

export const CANDIDATE_LIST = obj({
  schema_version: en('1'),
  scan_scope: str,
  rejected_count: int,
  notes: nstr,
  candidates: arr(candidate, 8),
});

// ---------------------------------------------------------------- task packet

const contract = obj({
  contract_id: str,
  statement: str,
  kind: en('SIGNATURE', 'RETURN_SHAPE', 'SIDE_EFFECT', 'ORDERING', 'ERROR_BEHAVIOR', 'RENDER_OUTPUT', 'PERSISTED_SHAPE'),
  verified_by: str,
});

export const TASK_PACKET = obj({
  schema_version: en('1'),
  candidate_id: str,
  category: en(...CATEGORIES),
  title: str,
  minimal_change: str,
  allowlist: arr(str),
  forbidden_files: arr(str),
  contracts: arr(contract),
  stop_conditions: arr(str),
  rollback: obj({
    strategy: en('GIT_RESET_HARD', 'DISCARD_WORKTREE'),
    detail: str,
  }),
  risk_level: en(...RISKS),
  characterization_needed: bool,
  characterization_files: arr(str),
  readiness: en('READY', 'NEEDS_EVIDENCE', 'REJECT'),
  readiness_reason: str,
  notes: nstr,
});

// ---------------------------------------------------------------- preflight

export const PREFLIGHT_RESULT = obj({
  schema_version: en('1'),
  candidate_id: str,
  verdict: en('READY_TO_EXECUTE', 'BLOCKED'),
  failure_hypothesis: str,
  falsification_method: str,
  falsification_result: en('SURVIVED', 'FALSIFIED', 'INCONCLUSIVE'),
  blocking_reasons: arr(obj({
    code: en('DYNAMIC_REFERENCE_FOUND', 'EXTERNAL_CONSUMER', 'NO_TEST_PROTECTION',
      'CONTRACT_UNCLEAR', 'SCOPE_TOO_LARGE', 'BASELINE_UNSTABLE', 'EVIDENCE_STALE'),
    detail: str,
    file: nstr,
  })),
  notes: nstr,
});

// ---------------------------------------------------------------- execution

/**
 * There is deliberately no `commands_run` field: the orchestrator owns command
 * execution, so there is nothing to ask the model about. `changed_files` is
 * cross-checked against `git status --porcelain` — a mismatch between what the
 * model says it changed and what git shows is the cheapest available detector
 * for a model that quietly wandered out of its lane.
 *
 * `rationale` exists for the one thing a diff genuinely cannot carry: WHY a
 * particular seam was chosen for a split. The reviewer receives it while still
 * being denied the verdict and the hunk classifications.
 *
 * `deleted_files` exists because no provider is given a deletion primitive.
 * claude's write tool set (Edit, Write) can overwrite a file's contents but
 * cannot remove a path, and Bash is denied — so it reported, correctly, that
 * DEAD_CODE removal was physically impossible. Rather than hand one provider a
 * shell, the model DECLARES the removals and the orchestrator performs them
 * under the same allowlist check. Provider-symmetric, and no new privilege.
 */
export const EXECUTION_RESULT = obj({
  schema_version: en('1'),
  candidate_id: str,
  changed_files: arr(str),
  deleted_files: arr(str),
  rationale: str,
  hunks: arr(obj({
    hunk_id: str,
    file: str,
    classification: en('INTERNAL_ONLY', 'PROVEN_EQUIVALENT', 'UNEXPLAINED', 'CONTRACT_CHANGING'),
    justification: str,
    contract_ids: arr(str),
  })),
  verdict: en('PASS', 'FAIL'),
  scope_expansion_required: bool,
  scope_expansion_reason: nstr,
  notes: nstr,
});

// ---------------------------------------------------------------- review

export const REVIEW_RESULT = obj({
  schema_version: en('1'),
  candidate_id: str,
  verdict: en('PASS', 'FAIL'),
  behavior_preservation_assessment: en('PRESERVED', 'NOT_PRESERVED', 'CANNOT_DETERMINE'),
  unreviewable_hunks: arr(str),
  findings: arr(obj({
    finding_id: str,
    severity: en('BLOCKER', 'HIGH', 'MEDIUM', 'LOW'),
    file: str,
    locator: str,
    claim: str,
    why_it_matters: str,
    suggested_action: str,
  })),
  notes: nstr,
});

// ---------------------------------------------------------------- characterization

export const CHARACTERIZATION_RESULT = obj({
  schema_version: en('1'),
  candidate_id: str,
  changed_files: arr(str),
  characterized_contracts: arr(str),
  production_source_changed: bool,
  assertions_weakened: bool,
  verdict: en('PASS', 'FAIL'),
  notes: nstr,
});

export const SCHEMAS = {
  audit: CANDIDATE_LIST,
  deepcheck: TASK_PACKET,
  preflight: PREFLIGHT_RESULT,
  characterization: CHARACTERIZATION_RESULT,
  execute: EXECUTION_RESULT,
  review: REVIEW_RESULT,
};

// ---------------------------------------------------------------- validator

/**
 * Enough JSON Schema to check what we actually emit. The provider enforces the
 * schema; this is the "trust but verify" pass, and its error strings are fed
 * verbatim into the single repair re-ask.
 * @returns {string[]} empty when valid
 */
export function validate(schema, value, at = '$') {
  const errs = [];
  const types = Array.isArray(schema.type) ? schema.type : [schema.type];
  const actual = value === null ? 'null' : Array.isArray(value) ? 'array' : typeof value;

  const typeOk = types.some((t) =>
    t === 'integer' ? Number.isInteger(value)
      : t === 'number' ? typeof value === 'number'
        : t === actual);
  if (!typeOk) {
    errs.push(`${at}: expected ${types.join('|')}, got ${actual}`);
    return errs;
  }

  if (schema.enum && !schema.enum.includes(value)) {
    errs.push(`${at}: ${JSON.stringify(value)} is not one of ${schema.enum.join('|')}`);
  }

  if (actual === 'object' && schema.properties) {
    for (const k of schema.required ?? []) {
      if (!(k in value)) errs.push(`${at}.${k}: required property is missing`);
    }
    if (schema.additionalProperties === false) {
      for (const k of Object.keys(value)) {
        if (!(k in schema.properties)) errs.push(`${at}.${k}: unexpected property`);
      }
    }
    for (const [k, sub] of Object.entries(schema.properties)) {
      if (k in value) errs.push(...validate(sub, value[k], `${at}.${k}`));
    }
  }

  if (actual === 'array' && schema.items) {
    value.forEach((v, i) => errs.push(...validate(schema.items, v, `${at}[${i}]`)));
  }
  return errs;
}

// ---------------------------------------------------------------- post-rules
//
// Verdicts the model does not get to decide. Each one exists because the model
// has an incentive to be optimistic and the loop acts on the answer.

/** An unexplained hunk in a behaviour-preserving refactor is the whole point of the loop. */
export function enforceExecutionVerdict(result, packet) {
  const reasons = [];
  const dirty = result.hunks.filter((h) => h.classification === 'UNEXPLAINED' || h.classification === 'CONTRACT_CHANGING');
  if (dirty.length > 0) reasons.push(`${dirty.length} hunk(s) classified ${[...new Set(dirty.map((h) => h.classification))].join('/')}`);
  if (result.scope_expansion_required) reasons.push('scope expansion required');
  const allow = new Set(packet?.allowlist ?? []);
  const claimed = [...(result.changed_files ?? []), ...(result.deleted_files ?? [])];
  const outside = claimed.filter((f) => !allow.has(f));
  if (packet && packet.category !== 'LARGE_COMPONENT_SPLIT' && outside.length > 0) {
    reasons.push(`claims changes outside the allowlist: ${outside.join(', ')}`);
  }
  return { verdict: reasons.length > 0 ? 'FAIL' : result.verdict, reasons };
}

/** INCONCLUSIVE is not a shrug; an unkilled objection means the plan is not cleared. */
export function enforcePreflightVerdict(result) {
  const reasons = [];
  if (result.falsification_result !== 'FALSIFIED') reasons.push(`falsification ${result.falsification_result}`);
  if ((result.blocking_reasons ?? []).length > 0) reasons.push(`${result.blocking_reasons.length} blocking reason(s)`);
  return { verdict: reasons.length > 0 ? 'BLOCKED' : result.verdict, reasons };
}

/** CANNOT_DETERMINE is a failure: an unverifiable behaviour-preserving refactor has no business merging. */
export function enforceReviewVerdict(result) {
  const reasons = [];
  const blockers = (result.findings ?? []).filter((f) => f.severity === 'BLOCKER');
  if (blockers.length > 0) reasons.push(`${blockers.length} BLOCKER finding(s)`);
  if (result.behavior_preservation_assessment !== 'PRESERVED') {
    reasons.push(`behaviour assessment ${result.behavior_preservation_assessment}`);
  }
  return { verdict: reasons.length > 0 ? 'FAIL' : result.verdict, reasons };
}

export function enforceCharacterizationVerdict(result) {
  const reasons = [];
  if (result.production_source_changed) reasons.push('production source was modified in a test-only slice');
  if (result.assertions_weakened) reasons.push('existing assertions were weakened');
  return { verdict: reasons.length > 0 ? 'FAIL' : result.verdict, reasons };
}
