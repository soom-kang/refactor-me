// provider.mjs — the two adapters, their failure taxonomy, and the provider ring.
//
// Design commitments, each paid for by a predecessor's failure:
//
//  * NO RESUME. sessionId is captured for forensics and never fed back. Resume
//    capability is precisely what makes a run un-handoffable, so the adapter
//    must not even be able to do it.
//  * claude gets NO Bash, in any phase. Its Bash is a pattern blocklist, not an
//    OS sandbox, and blocklists leak (`node -e`, `sh -c`, redirection). The
//    orchestrator runs every command instead — which also guarantees both
//    providers see byte-identical repository facts.
//  * codex gets exactly TWO `-c` overrides. The bar is "I measured a behavior
//    change without it". Every `-c` is a hard coupling to a config key that
//    moves between releases; a previous implementation set ~20 and fought them.
//  * Never classify failure from a codex JSONL `item.type:"error"` event. A
//    successful exit-0 run emits one for the benign "Skill descriptions were
//    shortened to fit the skills context budget" warning, which fires in any
//    repository with a large skill catalog — i.e. every target repository here.

import { spawn } from 'node:child_process';
import fs from 'node:fs';
import path from 'node:path';
import { validate } from './schemas.mjs';
import { repairPrompt, newNonce } from './prompts.mjs';

export const PROVIDERS = ['claude', 'codex'];

/**
 * Phases that may write. Everything else is read-only, INCLUDING any phase name
 * this table has never heard of: privilege must fail closed. An earlier version
 * inferred the mode from a set of read-phase names, which meant a typo or a new
 * phase silently ran with Edit/Write and a workspace-write sandbox. `mode` is
 * now stated explicitly on every request and cross-checked against this table.
 */
export const WRITE_PHASES = new Set(['characterization', 'execute']);

export function modeFor(phase) {
  return WRITE_PHASES.has(phase) ? 'write' : 'read';
}

/**
 * Reasoning effort per phase. Running everything at `high` is what makes a run
 * cost hours, and most of that spend buys nothing: preflight is a second opinion
 * on a packet that deep-check already gated, and characterization is writing
 * tests against code it can read. The three phases where a weaker answer
 * actually costs correctness keep the full budget:
 *   audit       — its quality determines every downstream phase
 *   deep_check  — the evidence gate that stands between the loop and a bad deletion
 *   execute     — the edit itself
 *   review      — the independent check on that edit
 * Overridable per agent in config as agents.<name>.effort_by_phase.
 */
export const PHASE_EFFORT = {
  audit: 'high',
  deep_check: 'high',
  preflight: 'medium',
  characterization: 'medium',
  execute: 'high',
  review: 'high',
  handoff: 'low',
  doctor: 'low',
};

export function effortFor(phase, agent = {}) {
  return agent.effort_by_phase?.[phase] ?? agent.effort ?? PHASE_EFFORT[phase] ?? 'medium';
}

const DEFAULT_TIMEOUT_MS = 1_800_000;

// ---------------------------------------------------------------- argv

export function claudeArgs({ mode, schema, model, effort, maxBudgetUsd }) {
  const a = ['-p', '--output-format', 'stream-json', '--verbose'];
  if (schema) a.push('--json-schema', JSON.stringify(schema));

  // `Skill` must be in the tool set or /skill-name does nothing: restricting
  // --tools without it silently disables the entire routing layer while every
  // phase still appears to succeed. Verified live — a session without it reports
  // no available skills at all.
  if (mode === 'write') {
    a.push('--permission-mode', 'acceptEdits');
    a.push('--tools', 'Skill,Read,Glob,Grep,Edit,Write');
    a.push('--disallowed-tools', 'Bash', 'NotebookEdit', 'WebFetch', 'WebSearch');
  } else {
    a.push('--permission-mode', 'plan');
    a.push('--tools', 'Skill,Read,Glob,Grep');
    a.push('--disallowed-tools', 'Edit', 'Write', 'NotebookEdit', 'Bash', 'WebFetch', 'WebSearch');
  }

  // We WANT project skills to load, so --setting-sources project is the correct
  // isolation knob. --safe-mode and --bare both disable skills outright (and
  // --bare additionally forces API-key-only auth, fatal for a subscription
  // user), and --disable-slash-commands would silently neuter the whole routing
  // layer, since skills are invoked as /skill-name in the prompt body.
  a.push('--setting-sources', 'project');
  a.push('--strict-mcp-config', '--mcp-config', '{"mcpServers":{}}');
  a.push('--no-session-persistence', '--no-chrome');
  if (model) a.push('--model', model);
  if (effort) a.push('--effort', effort);
  if (maxBudgetUsd && maxBudgetUsd > 0) a.push('--max-budget-usd', String(maxBudgetUsd));
  return a;
}

export function codexArgs({ mode, cwd, schemaPath, outPath, model, effort }) {
  const a = ['exec', '-'];
  a.push('--sandbox', mode === 'write' ? 'workspace-write' : 'read-only');
  a.push('--cd', cwd);
  if (schemaPath) a.push('--output-schema', schemaPath);
  a.push('--output-last-message', outPath);
  a.push('--json');
  // Removes hooks and plugin skill caches while PRESERVING project
  // .agents/skills, and trims a few thousand tokens of baseline context.
  a.push('--ignore-user-config', '--ignore-rules');
  // --ignore-user-config resets reasoning effort to "none"; without this the
  // isolation silently downgrades the model. This is why the list is not empty.
  if (model) a.push('-c', `model="${model}"`);
  a.push('-c', `model_reasoning_effort="${effort || 'high'}"`);
  // No --skip-git-repo-check: the workspace is always a worktree, so letting
  // codex enforce its own git check is a free assertion that we are where we
  // think we are.
  return a;
}

// ---------------------------------------------------------------- spawn

/**
 * Provider processes get a sanitized environment rather than this process's.
 * An inherited environment is not neutral: it carries whatever the parent shell
 * or agent session was configured with, and a provider CLI is entitled to read
 * its own configuration from it. Passing only what a CLI genuinely needs keeps
 * a run reproducible and stops the surrounding session from silently changing
 * how the sandbox behaves.
 */
export function sanitizedEnv() {
  const keep = [
    'PATH', 'HOME', 'USER', 'LOGNAME', 'SHELL', 'TMPDIR', 'LANG', 'LC_ALL', 'TERM',
    'XDG_CONFIG_HOME', 'XDG_CACHE_HOME',
    // provider credentials and endpoints, without which nothing can authenticate
    'ANTHROPIC_API_KEY', 'ANTHROPIC_AUTH_TOKEN', 'ANTHROPIC_BASE_URL', 'CLAUDE_CONFIG_DIR',
    'OPENAI_API_KEY', 'OPENAI_BASE_URL', 'CODEX_HOME',
    // toolchains the validation commands need
    'NVM_DIR', 'NODE_PATH', 'GOPATH', 'GOROOT', 'GOMODCACHE', 'CARGO_HOME', 'RUSTUP_HOME',
    'PYENV_ROOT', 'VIRTUAL_ENV', 'JAVA_HOME', 'BUN_INSTALL', 'PNPM_HOME',
  ];
  const env = {};
  for (const k of keep) if (process.env[k] !== undefined) env[k] = process.env[k];
  env.FORCE_COLOR = '0';
  env.NO_COLOR = '1';
  env.CI = '1';
  return env;
}

function runProcess(bin, args, { cwd, input, timeoutMs, onEvent }) {
  return new Promise((resolve) => {
    const child = spawn(bin, args, {
      cwd,
      env: sanitizedEnv(),
      detached: true,
      stdio: ['pipe', 'pipe', 'pipe'],
    });
    let stdout = '', stderr = '', killed = false, killer = null, buf = '';

    child.stdout.on('data', (b) => {
      const s = b.toString();
      stdout += s;
      if (!onEvent) return;
      buf += s;
      const parts = buf.split('\n');
      buf = parts.pop() ?? '';
      for (const line of parts) {
        if (!line.trim()) continue;
        try { onEvent(JSON.parse(line)); } catch { /* non-JSON banner lines */ }
      }
    });
    child.stderr.on('data', (b) => { stderr += b.toString(); });

    const timer = setTimeout(() => {
      killed = true;
      try { process.kill(-child.pid, 'SIGTERM'); } catch { /* gone */ }
      killer = setTimeout(() => { try { process.kill(-child.pid, 'SIGKILL'); } catch { /* gone */ } }, 10_000);
    }, timeoutMs ?? DEFAULT_TIMEOUT_MS);

    child.on('error', (e) => { stderr += String(e.message); });
    child.on('close', (code) => {
      clearTimeout(timer); if (killer) clearTimeout(killer);
      resolve({ stdout, stderr, exitCode: killed ? null : code, killed });
    });

    child.stdin.end(input ?? '');
  });
}

// ---------------------------------------------------------------- parsing

export function stripFences(text) {
  const t = text.trim();
  const m = /^```(?:json)?\s*\n([\s\S]*?)\n?```$/.exec(t);
  return (m ? m[1] : t).trim();
}

/** Last complete top-level JSON object in a blob, tolerating leading banners. */
export function lastJsonObject(text) {
  let depth = 0, start = -1, best = null, inStr = false, esc = false;
  for (let i = 0; i < text.length; i++) {
    const c = text[i];
    if (inStr) { if (esc) esc = false; else if (c === '\\') esc = true; else if (c === '"') inStr = false; continue; }
    if (c === '"') { inStr = true; continue; }
    if (c === '{') { if (depth === 0) start = i; depth++; }
    else if (c === '}') { depth--; if (depth === 0 && start >= 0) best = text.slice(start, i + 1); }
  }
  if (!best) return null;
  try { return JSON.parse(best); } catch { return null; }
}

/** Human-readable one-liners for the live log, from either provider's stream. */
function describeEvent(ev) {
  // claude stream-json
  if (ev.type === 'assistant' && ev.message?.content) {
    for (const c of ev.message.content) {
      if (c.type !== 'tool_use') continue;
      const i = c.input ?? {};
      if (c.name === 'Read') return `read ${i.file_path ?? ''}`;
      if (c.name === 'Grep') return `grep ${JSON.stringify(i.pattern ?? '')}`;
      if (c.name === 'Glob') return `glob ${i.pattern ?? ''}`;
      if (c.name === 'Edit') return `edit ${i.file_path ?? ''}`;
      if (c.name === 'Write') return `write ${i.file_path ?? ''}`;
      if (c.name === 'Skill') return `skill ${i.skill ?? i.name ?? ''}`;
      return `tool ${c.name}`;
    }
  }
  // codex --json
  if (ev.type === 'item.completed' && ev.item) {
    const it = ev.item;
    if (it.type === 'command_execution') return `run ${String(it.command ?? '').slice(0, 70)}`;
    if (it.type === 'file_change') return `edit ${(it.changes ?? []).map((c) => c.path).join(', ').slice(0, 70)}`;
    if (it.type === 'error') return null;   // benign warnings wear this shape; never surface as failure
  }
  return null;
}

function parseClaude(stdout) {
  // The final `type:"result"` event carries the same envelope as --output-format json.
  let env = null;
  for (const line of stdout.split('\n')) {
    if (!line.trim()) continue;
    let ev; try { ev = JSON.parse(line); } catch { continue; }
    if (ev.type === 'result') env = ev;
  }
  if (!env) env = lastJsonObject(stdout);
  if (!env) return null;
  return {
    env,
    text: typeof env.result === 'string' ? env.result : '',
    // The payload is a TOP-LEVEL field, a sibling of `result` — not nested
    // inside it and not JSON embedded in the result string.
    data: env.structured_output ?? null,
    sessionId: env.session_id ?? null,
    usage: {
      // The two CLIs mean different things by `input_tokens`. codex reports a
      // total with `cached_input_tokens` as a subset of it; claude reports only
      // the uncached remainder and puts the rest in cache_read/cache_creation.
      // Taken literally, a claude call that read 292k tokens reports 18, which
      // would make any published token figure nonsense. Summed here so the two
      // providers mean the same thing by the word.
      inputTokens: (env.usage?.input_tokens ?? 0)
        + (env.usage?.cache_read_input_tokens ?? 0)
        + (env.usage?.cache_creation_input_tokens ?? 0),
      outputTokens: env.usage?.output_tokens ?? 0,
      costUsd: env.total_cost_usd ?? null,
    },
    permissionDenials: env.permission_denials ?? [],
  };
}

function parseCodex(stdout, outPath) {
  let raw = '';
  try { raw = fs.readFileSync(outPath, 'utf8'); } catch { /* absent */ }
  let data = null;
  if (raw.trim()) { try { data = JSON.parse(stripFences(raw)); } catch { data = null; } }
  let sessionId = null, usage = { inputTokens: 0, outputTokens: 0, costUsd: null };
  for (const line of stdout.split('\n')) {
    if (!line.trim()) continue;
    let ev; try { ev = JSON.parse(line); } catch { continue; }
    if (ev.type === 'thread.started') sessionId = ev.thread_id ?? sessionId;
    if (ev.type === 'turn.completed' && ev.usage) {
      usage = { inputTokens: ev.usage.input_tokens ?? 0, outputTokens: ev.usage.output_tokens ?? 0, costUsd: null };
    }
  }
  return { env: null, text: raw, data, sessionId, usage, permissionDenials: [] };
}

// ---------------------------------------------------------------- usage

/** A zero accumulator. `costUsd: null` means "nothing reported", not "$0". */
export function newUsage() {
  return { processes: 0, ms: 0, inputTokens: 0, outputTokens: 0, costUsd: null, costMissing: 0, failedCalls: 0 };
}

/**
 * Add two usage records. The whole point is the null handling: codex never
 * reports a turn cost (parseCodex pins costUsd to null), so a run that used it
 * has a genuinely unknown total. Folding null to 0 would render that as $0.00 —
 * a number a reader would believe. Instead null + null stays null, and
 * costMissing counts how many contributions could not be priced, so the caller
 * can say "$2.41 or more" rather than lying in either direction.
 *
 * Accepts both raw per-process records (no costMissing) and accumulators.
 */
export function mergeUsage(a, b) {
  const missing = (u) => u.costMissing ?? (u.costUsd == null ? 1 : 0);
  const bothNull = a.costUsd == null && b.costUsd == null;
  return {
    processes: (a.processes ?? 1) + (b.processes ?? 1),
    ms: (a.ms ?? 0) + (b.ms ?? 0),
    inputTokens: (a.inputTokens ?? 0) + (b.inputTokens ?? 0),
    outputTokens: (a.outputTokens ?? 0) + (b.outputTokens ?? 0),
    costUsd: bothNull ? null : (a.costUsd ?? 0) + (b.costUsd ?? 0),
    costMissing: missing(a) + missing(b),
    failedCalls: (a.failedCalls ?? 0) + (b.failedCalls ?? 0),
  };
}

/** "812k" / "41.9k" / "412" — meant to be read at a glance, not audited. */
export function fmtTokens(n) {
  if (!Number.isFinite(n)) return '0';
  if (n < 1000) return String(n);
  if (n < 100_000) return `${(n / 1000).toFixed(1)}k`;
  return `${Math.round(n / 1000)}k`;
}

/** "4m12s" / "38s" */
export function fmtMs(ms) {
  const s = Math.round((ms ?? 0) / 1000);
  return s < 60 ? `${s}s` : `${Math.floor(s / 60)}m${String(s % 60).padStart(2, '0')}s`;
}

/**
 * How to say what a usage record cost. Never "$0.00" for a provider that simply
 * does not report — that is a number a reader would believe.
 *   nothing priced      → { text: null, partial: false }
 *   some of it priced   → { text: '$2.41', partial: true }   ("or more")
 *   all of it priced    → { text: '$2.41', partial: false }
 */
export function costOf(u) {
  if (!u || u.costUsd == null) return { text: null, partial: (u?.costMissing ?? 0) > 0 };
  return { text: `$${u.costUsd.toFixed(2)}`, partial: (u.costMissing ?? 0) > 0 };
}

/** One live line: `audit · claude · 4m12s · 38.2k↓ / 2.1k↑ · $0.41` */
export function usageLine(phase, provider, { usage, durationMs }) {
  const c = costOf(usage);
  const cost = c.text ? (c.partial ? `${c.text}+` : c.text) : 'cost n/a';
  return `${phase} · ${provider} · ${fmtMs(durationMs)} · ${fmtTokens(usage?.inputTokens ?? 0)}↓ / ${fmtTokens(usage?.outputTokens ?? 0)}↑ · ${cost}`;
}

// ---------------------------------------------------------------- taxonomy

// Classification order matters: the vendors' own strings overlap, so the
// unambiguous signals are resolved first.
const QUOTA_HARD_RE = /usage limit reached|hit your usage limit|credit balance (is )?too low|out of usage credits|monthly spend limit|insufficient_quota|upgrade to increase your usage limit|weekly limit|resource_exhausted/i;
const AUTH_RE = /failed to authenticate|oauth session expired|invalid api key|invalid_api_key|please run \/login|codex login|not logged in|authentication_error|unauthorized|\b401\b|missing (openai_api_key|api key)/i;
const QUOTA_SOFT_RE = /rate.?limit|too many requests|overloaded|\b429\b|\b529\b/i;

export function classifyClaude({ stdout, stderr, killed, schemaRequested }) {
  if (killed) return { failure: 'TIMEOUT', detail: 'killed after timeout' };
  const p = parseClaude(stdout);
  if (!p) {
    if (/ENOENT|command not found/i.test(stderr)) return { failure: 'PROCESS', detail: 'cli_missing', fatal: true };
    return { failure: 'PROCESS', detail: 'unparseable stdout' };
  }
  const env = p.env ?? {};
  const msg = `${env.result ?? ''} ${stderr}`;

  // claude retries structured output internally; by the time we see this it has
  // already spent more than one attempt, which is why our repair budget is one.
  if (env.subtype === 'error_max_structured_output_retries' ||
      env.terminal_reason === 'structured_output_retry_exhausted') {
    return { failure: 'SCHEMA', detail: 'provider retries exhausted', parsed: p };
  }

  // NEVER branch on subtype for success: subtype:"success" coexists with is_error:true.
  if (env.is_error === true) {
    if (QUOTA_HARD_RE.test(msg)) return { failure: 'QUOTA', hard: true, detail: firstLine(msg), parsed: p };
    if (AUTH_RE.test(msg) || env.api_error_status === 401) return { failure: 'AUTH', detail: firstLine(msg), parsed: p };
    if (QUOTA_SOFT_RE.test(msg) || env.api_error_status === 429 || env.api_error_status === 529) {
      return { failure: 'QUOTA', hard: false, detail: firstLine(msg), parsed: p };
    }
    return { failure: 'PROCESS', detail: env.terminal_reason ?? firstLine(msg) ?? 'unknown', parsed: p };
  }

  if (schemaRequested && p.data == null) {
    return { failure: 'SCHEMA', detail: 'no structured_output on a successful turn', parsed: p };
  }
  return { failure: 'OK', parsed: p };
}

export function classifyCodex({ stdout, stderr, exitCode, killed, outPath, schemaRequested }) {
  if (killed) return { failure: 'TIMEOUT', detail: 'killed after timeout' };
  const msg = `${stderr}\n${stdout}`;

  // Our bug, not the model's: codex rejected the schema file, so the model never
  // ran. Retrying, repairing and switching providers are all wrong answers.
  if (exitCode !== 0 && /output[- ]schema|invalid schema|schema file|failed to parse schema/i.test(stderr)) {
    return { failure: 'PROCESS', detail: 'schema file rejected by codex', fatal: true };
  }
  if (exitCode !== 0) {
    if (QUOTA_HARD_RE.test(msg)) return { failure: 'QUOTA', hard: true, detail: firstLine(stderr) };
    if (AUTH_RE.test(msg)) return { failure: 'AUTH', detail: firstLine(stderr) };
    if (QUOTA_SOFT_RE.test(msg)) return { failure: 'QUOTA', hard: false, detail: firstLine(stderr) };
    if (/ENOENT|command not found|No such file/i.test(msg)) return { failure: 'PROCESS', detail: 'cli_missing', fatal: true };
    return { failure: 'PROCESS', detail: firstLine(stderr) ?? `exit ${exitCode}` };
  }

  // exit 0 — the only remaining failure mode is the payload itself.
  const p = parseCodex(stdout, outPath);
  if (schemaRequested) {
    if (!p.text.trim()) return { failure: 'SCHEMA', detail: 'empty last message', parsed: p };
    if (p.data == null) return { failure: 'SCHEMA', detail: 'last message is not valid JSON', parsed: p };
  }
  return { failure: 'OK', parsed: p };
}

const firstLine = (s) => (s ?? '').split('\n').map((l) => l.trim()).find(Boolean) ?? null;

// ---------------------------------------------------------------- adapter

/**
 * One provider call. Returns the AgentResult shape; the caller decides what a
 * failure means for the loop.
 */
export async function callProvider(provider, req, cfg, log) {
  // Fail closed: an unstated mode is read, and a request that claims write for a
  // phase the table does not allow to write is a programming error, not a
  // permission to grant.
  const mode = req.mode ?? 'read';
  if (mode !== 'read' && mode !== 'write') throw new Error(`invalid provider mode: ${mode}`);
  if (mode === 'write' && !WRITE_PHASES.has(req.phase)) {
    throw new Error(`phase "${req.phase}" requested write mode but is not in WRITE_PHASES`);
  }
  const agent = cfg.agents[provider] ?? {};
  const bin = agent.bin || provider;
  const timeoutMs = req.timeoutMs ?? (agent.timeout_sec ?? 1800) * 1000;
  let tools = 0;

  let args, input, outPath = null, schemaPath = null;
  if (provider === 'claude') {
    args = claudeArgs({
      mode, schema: req.schema, model: agent.model || undefined,
      effort: req.effort ?? effortFor(req.phase, agent),
      maxBudgetUsd: agent.max_budget_usd ?? 0,
    });
    input = req.body;
  } else {
    // Outside the worktree by design: the codex process writes this itself, so
    // it never has to be hidden from, or filtered out of, the change set.
    outPath = path.join(req.runDir, 'provider', `${req.phase}-last.json`);
    fs.mkdirSync(path.dirname(outPath), { recursive: true });
    fs.rmSync(outPath, { force: true });
    if (req.schema) {
      schemaPath = path.join(req.runDir, 'schemas', `${req.phase}.json`);
      fs.mkdirSync(path.dirname(schemaPath), { recursive: true });
      fs.writeFileSync(schemaPath, JSON.stringify(req.schema, null, 2));
    }
    args = codexArgs({
      mode, cwd: req.cwd, schemaPath, outPath,
      model: agent.model || undefined, effort: req.effort ?? effortFor(req.phase, agent),
    });
    input = req.body;
  }

  const startedAt = Date.now();
  log?.beginHeartbeat(() => `${tools} tool call${tools === 1 ? '' : 's'}`);
  const res = await runProcess(bin, args, {
    cwd: req.cwd, input, timeoutMs,
    onEvent: (ev) => {
      const d = describeEvent(ev);
      if (d) { tools += 1; log?.info(d); }
    },
  });
  const durationMs = Date.now() - startedAt;
  log?.endHeartbeat();

  // Raw transcripts stay under .refactor/, which is gitignored and never
  // committed. We never echo process.env and never log .env* contents.
  const rawBase = path.join(req.runDir, 'provider', `${req.phase}-${provider}-${req.attempt}`);
  fs.mkdirSync(path.dirname(rawBase), { recursive: true });
  fs.writeFileSync(`${rawBase}.prompt.md`, req.body);
  fs.writeFileSync(`${rawBase}.stdout.jsonl`, res.stdout);
  if (res.stderr.trim()) fs.writeFileSync(`${rawBase}.stderr.log`, res.stderr);

  const cls = provider === 'claude'
    ? classifyClaude({ ...res, schemaRequested: Boolean(req.schema) })
    : classifyCodex({ ...res, outPath, schemaRequested: Boolean(req.schema) });

  const p = cls.parsed ?? { text: '', data: null, sessionId: null, usage: { inputTokens: 0, outputTokens: 0, costUsd: null }, permissionDenials: [] };

  // A read phase whose model tried to write is not a failure, but it is a
  // prompt-quality signal worth keeping.
  if (mode === 'read' && p.permissionDenials?.length) {
    log?.warn(`${provider} attempted ${p.permissionDenials.length} denied write(s) in a read phase`);
  }

  return {
    ok: cls.failure === 'OK',
    failure: cls.failure,
    hard: cls.hard ?? false,
    fatal: cls.fatal ?? false,
    detail: cls.detail ?? null,
    data: p.data,
    text: p.text,
    provider,
    sessionId: p.sessionId,
    exitCode: res.exitCode,
    usage: p.usage,
    rawPath: `${rawBase}.stdout.jsonl`,
    toolCalls: tools,
    // Always present, including on TIMEOUT and PROCESS: a call that burned
    // thirty minutes and produced nothing still spent thirty minutes.
    durationMs,
    processes: 1,
  };
}

/**
 * A provider call plus, on a schema failure only, exactly one bounded repair
 * re-ask. The repair session is deliberately degraded to read-only regardless of
 * the original phase: it must reshape output, not redo work.
 */
export async function callWithRepair(provider, req, cfg, log) {
  let res = await callProvider(provider, { ...req, attempt: 'primary' }, cfg, log);
  if (res.ok && req.schema) {
    const errs = validate(req.schema, res.data);
    if (errs.length === 0) return res;
    res = { ...res, ok: false, failure: 'SCHEMA', detail: errs.slice(0, 5).join('; ') };
  }
  if (res.failure !== 'SCHEMA' || res.fatal) return res;

  log?.retry(`${provider} schema failure (${res.detail}) → one repair attempt`);
  const repair = await callProvider(provider, {
    ...req,
    phase: `${req.phase}`,
    attempt: 'repair',
    body: repairPrompt({ nonce: newNonce(), errors: res.detail ?? 'unspecified', priorText: res.text || '(empty)' }),
  }, cfg, log);

  // The repair is a second CLI process, and the first one's tokens were already
  // spent. Returning `repair` alone silently dropped them from every total, so
  // a run that repaired once under-reported itself and the docs built on those
  // numbers would be wrong.
  const withFirst = (r) => {
    const u = mergeUsage(res.usage, r.usage);
    return {
      ...r,
      // token/cost only: `processes` is counted once, at the top level.
      usage: { inputTokens: u.inputTokens, outputTokens: u.outputTokens, costUsd: u.costUsd, costMissing: u.costMissing },
      durationMs: (res.durationMs ?? 0) + (r.durationMs ?? 0),
      processes: (res.processes ?? 1) + (r.processes ?? 1),
    };
  };

  if (repair.ok && req.schema) {
    const errs = validate(req.schema, repair.data);
    if (errs.length > 0) return withFirst({ ...repair, ok: false, failure: 'SCHEMA', detail: errs.slice(0, 5).join('; ') });
  }
  return withFirst(repair);
}

// ---------------------------------------------------------------- ring

/**
 * Provider selection. Switching happens ONLY on quota or auth — after a
 * successful cycle the loop stays where it is, because ping-ponging spends two
 * budgets to do one job.
 */
/**
 * Ready providers, in the order the operator asked for.
 *
 * `state.providerOrder` is the resolved `--provider` / `--fallback` order. Without
 * it this iterated the PROVIDERS array, whose order is alphabetical accident —
 * so `--provider codex` silently led with claude, and the flag did nothing on the
 * first call of a run.
 */
export function readyProviders(state, now = Date.now()) {
  const out = [];
  for (const name of (state.providerOrder?.length ? state.providerOrder : PROVIDERS)) {
    const p = state.providers[name];
    if (!p || p.status === 'DISABLED' || p.status === 'DEAD') continue;
    if (p.status === 'COOLDOWN') {
      if (p.cooldownUntil && Date.parse(p.cooldownUntil) > now) continue;
      p.status = 'READY'; p.cooldownUntil = null;      // cooldown expired
    }
    out.push(name);
  }
  return out;
}

export function pickProvider(state, { prefer, exclude } = {}) {
  const ready = readyProviders(state).filter((n) => n !== exclude);
  if (ready.length === 0) return null;
  if (prefer && ready.includes(prefer)) return prefer;
  if (state.activeProvider && ready.includes(state.activeProvider)) return state.activeProvider;
  return ready[0];
}

/** The reviewer should be the provider that did NOT write, when one is available. */
export function pickReviewer(state, writer) {
  const other = PROVIDERS.find((n) => n !== writer);
  const ready = readyProviders(state);
  return ready.includes(other) ? other : (ready.includes(writer) ? writer : null);
}

export function noteFailure(state, provider, res, cooldownMinutes = 20) {
  const p = state.providers[provider];
  if (!p) return;
  p.lastError = res.detail ?? res.failure;
  if (res.failure === 'QUOTA') {
    p.quotaHits += 1;
    if (res.hard) {
      p.status = 'COOLDOWN';
      p.cooldownUntil = new Date(Date.now() + cooldownMinutes * 60_000).toISOString();
    }
  } else if (res.failure === 'AUTH') {
    p.status = 'DEAD';
  }
}

export function noteCall(state, provider) {
  const p = state.providers[provider];
  if (p) p.calls += 1;
  state.activeProvider = provider;
}

/**
 * Record what one callWithRepair consumed, on both axes the report needs: per
 * provider — where the cost asymmetry lives, since claude reports a turn cost
 * and codex never does — and per phase, which is the axis that answers "why was
 * this run expensive".
 *
 * Failed calls count. A timeout spent its thirty minutes and a quota-killed
 * call may still have burned tokens; the question here is what the run cost,
 * not what it cost to succeed. `failedCalls` keeps the two distinguishable.
 *
 * Separate from noteCall on purpose: that one runs BEFORE the call, to mark the
 * active provider, and cannot know what came back.
 */
export function noteUsage(state, { provider, phase, usage, durationMs = 0, processes = 1, ok = true }) {
  const one = {
    processes,
    ms: durationMs,
    inputTokens: usage?.inputTokens ?? 0,
    outputTokens: usage?.outputTokens ?? 0,
    costUsd: usage?.costUsd ?? null,
    costMissing: usage?.costMissing ?? (usage?.costUsd == null ? processes : 0),
    failedCalls: ok ? 0 : 1,
  };

  const ring = state.providers?.[provider];
  if (ring) ring.usage = mergeUsage(ring.usage ?? newUsage(), one);

  state.usage ??= { byPhase: {} };
  const prior = state.usage.byPhase[phase] ?? { ...newUsage(), calls: 0 };
  state.usage.byPhase[phase] = { ...mergeUsage(prior, one), calls: (prior.calls ?? 0) + 1 };
}
