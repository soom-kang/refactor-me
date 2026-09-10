#!/usr/bin/env node
// make-fixture.mjs — build a small, self-contained git repo that exercises every
// path the loop cares about, with ZERO npm dependencies so `npm test` etc. work
// on a machine with no network.
//
// Planted deliberately:
//   src/legacy-parser.mjs  truly unreferenced        -> should be removed
//   src/plugin-x.mjs       reachable ONLY by string  -> TRAP, must survive
//   src/status-{a,b,c}.mjs same map copied 3x        -> deduplication candidate
//   src/big-panel.mjs      620 lines with clean seams-> split candidate
//   src/compat.mjs         v1 payload shim           -> compatibility removal
//   src/broken.mjs         one permanent type error  -> RED baseline, differential path
//
// Usage: node make-fixture.mjs [dest] [--skills <dir>] [--multi]
//
// --multi adds app/web (bun + Next-ish) and app/api (Go) alongside the root,
// so area detection, per-area command discovery and the area-scoped validation
// ladder are exercised against real toolchains rather than only unit tests.
// The single-area default stays the fast path.

import fs from 'node:fs';
import path from 'node:path';
import { spawnSync } from 'node:child_process';

const dest = path.resolve(process.argv[2] ?? '/tmp/rl-fixture');
const argv = process.argv.slice(3);

function w(rel, body) {
  const f = path.join(dest, rel);
  fs.mkdirSync(path.dirname(f), { recursive: true });
  fs.writeFileSync(f, body.startsWith('\n') ? body.slice(1) : body);
}
function sh(args, cwd = dest) {
  const r = spawnSync(args[0], args.slice(1), { cwd, encoding: 'utf8' });
  if (r.status !== 0) throw new Error(`${args.join(' ')}\n${r.stderr}`);
  return r.stdout;
}

if (fs.existsSync(dest)) fs.rmSync(dest, { recursive: true, force: true });
fs.mkdirSync(dest, { recursive: true });

// ------------------------------------------------------------------ manifest

w('package.json', `
{
  "name": "rl-fixture",
  "version": "1.0.0",
  "private": true,
  "type": "module",
  "scripts": {
    "lint": "node tools/lint.mjs",
    "typecheck": "node tools/typecheck.mjs",
    "test": "node --test test/*.test.mjs",
    "build": "node tools/build.mjs"
  }
}
`);

w('.gitignore', `
node_modules/
dist/
.env.local
`);

// ------------------------------------------------------------------ tooling

w('tools/lint.mjs', `
// Zero-dependency lint: flags \`var\` and trailing whitespace under src/.
import fs from 'node:fs';
import path from 'node:path';

const root = path.resolve(import.meta.dirname, '..', 'src');
let bad = 0;
for (const f of fs.readdirSync(root)) {
  if (!f.endsWith('.mjs')) continue;
  const rel = \`src/\${f}\`;
  const text = fs.readFileSync(path.join(root, f), 'utf8');
  text.split('\\n').forEach((line, i) => {
    if (/^\\s*var\\s/.test(line)) { console.error(\`\${rel}:\${i + 1}: error L0001: unexpected \\\`var\\\`\`); bad++; }
    if (/[ \\t]+$/.test(line)) { console.error(\`\${rel}:\${i + 1}: error L0002: trailing whitespace\`); bad++; }
  });
}
console.log(bad === 0 ? 'lint: clean' : \`lint: \${bad} problem(s)\`);
process.exit(bad === 0 ? 0 : 1);
`);

w('tools/typecheck.mjs', `
// Zero-dependency stand-in type checker. Any line carrying the token
// UNTYPED_MARKER is reported as a hard error, with a tsc-shaped message so the
// orchestrator's signature extraction has something realistic to parse.
import fs from 'node:fs';
import path from 'node:path';

const root = path.resolve(import.meta.dirname, '..', 'src');
let bad = 0;
for (const f of fs.readdirSync(root)) {
  if (!f.endsWith('.mjs')) continue;
  const rel = \`src/\${f}\`;
  const text = fs.readFileSync(path.join(root, f), 'utf8');
  text.split('\\n').forEach((line, i) => {
    if (line.includes('UNTYPED_MARKER')) {
      console.error(\`\${rel}:\${i + 1}: error TS2345: Argument of type 'unknown' is not assignable to parameter of type 'string'.\`);
      bad++;
    }
  });
}
if (bad === 0) console.log('typecheck: clean');
process.exit(bad === 0 ? 0 : 1);
`);

w('tools/build.mjs', `
// Zero-dependency "build": import the entrypoint and emit a manifest.
import fs from 'node:fs';
import path from 'node:path';

const root = path.resolve(import.meta.dirname, '..');
const mod = await import(path.join(root, 'src', 'index.mjs'));
if (typeof mod.render !== 'function') { console.error('build: error B0001: missing export render'); process.exit(1); }
fs.mkdirSync(path.join(root, 'dist'), { recursive: true });
fs.writeFileSync(path.join(root, 'dist', 'manifest.json'), JSON.stringify({ exports: Object.keys(mod).sort() }, null, 2));
console.log('build: ok');
`);

// ------------------------------------------------------------------ src

w('src/index.mjs', `
import { renderPanel } from './big-panel.mjs';
import { resolvePlugin } from './registry.mjs';
import { readPayload } from './compat.mjs';
import { describe } from './broken.mjs';

export function render(state) {
  return renderPanel(state);
}

export function plugin(name) {
  return resolvePlugin(name);
}

export function ingest(raw) {
  return readPayload(raw);
}

export function label(x) {
  return describe(x);
}
`);

// --- TRUE dead code: imported by nobody, referenced by no string.
w('src/legacy-parser.mjs', `
// Superseded by compat.mjs in v2. Nothing imports this module and no string in
// the repository names it.
function tokenize(input) {
  return String(input).split(/[\\s,]+/).filter(Boolean);
}

export function parseLegacy(input) {
  const parts = tokenize(input);
  const out = {};
  for (let i = 0; i < parts.length; i += 2) out[parts[i]] = parts[i + 1] ?? null;
  return out;
}

export function legacyVersion() {
  return 1;
}
`);

// --- TRAP: only reachable through a string key. Deleting it breaks plugin().
w('src/plugin-x.mjs', `
export const id = 'plugin-x';

export function run(input) {
  return { id, value: String(input).toUpperCase() };
}
`);

w('src/registry.mjs', `
// Plugins are resolved by NAME, not by a static import. A reference search for
// "plugin-x.mjs" finds nothing; the only link is the string below.
const PLUGIN_NAMES = ['plugin-x'];

export async function resolvePlugin(name) {
  if (!PLUGIN_NAMES.includes(name)) return null;
  const mod = await import(\`./\${name}.mjs\`);
  return mod;
}

export function knownPlugins() {
  return [...PLUGIN_NAMES];
}
`);

// --- Duplication: the same status map, copied three times.
const statusMap = `
export const STATUS = {
  active: 'Active',
  pending: 'Pending review',
  suspended: 'Suspended',
  closed: 'Closed',
};

export function statusLabel(key) {
  return STATUS[key] ?? 'Unknown';
}
`;
w('src/status-a.mjs', `// Account view.\n${statusMap}`);
w('src/status-b.mjs', `// Admin view.\n${statusMap}`);
w('src/status-c.mjs', `// Export view.\n${statusMap}`);

// --- Compatibility shim for a payload version nothing produces any more.
w('src/compat.mjs', `
// v1 payloads used { user_name }. Since v2 everything emits { userName }.
// The v1 branch is retained for old records.
export function readPayload(raw) {
  const data = typeof raw === 'string' ? JSON.parse(raw) : raw;
  if (data && typeof data.user_name === 'string') {
    return { userName: data.user_name, version: 1 };
  }
  return { userName: data?.userName ?? '', version: 2 };
}
`);

// --- Permanent type error: keeps the baseline RED so the differential path runs.
w('src/broken.mjs', `
export function describe(value) {
  // UNTYPED_MARKER: intentional, permanent checker complaint for the fixture.
  return String(value);
}
`);

// --- 620-line component with visible seams.
{
  const L = [];
  L.push("import { statusLabel } from './status-a.mjs';");
  L.push('');
  L.push('// ---------------------------------------------------------------- helpers');
  for (let i = 1; i <= 18; i++) {
    L.push(`function formatField${i}(value) {`);
    L.push(`  if (value == null) return '';`);
    L.push(`  return String(value).trim().slice(0, ${20 + i});`);
    L.push('}');
    L.push('');
  }
  L.push('// ---------------------------------------------------------------- derivation');
  for (let i = 1; i <= 18; i++) {
    L.push(`function deriveMetric${i}(state) {`);
    L.push(`  const base = Number(state?.metrics?.m${i} ?? 0);`);
    L.push(`  const scaled = base * ${i};`);
    L.push(`  return Number.isFinite(scaled) ? scaled : 0;`);
    L.push('}');
    L.push('');
  }
  L.push('// ---------------------------------------------------------------- sections');
  for (let i = 1; i <= 18; i++) {
    L.push(`function renderSection${i}(state) {`);
    L.push(`  const label = formatField${i}(state?.fields?.f${i});`);
    L.push(`  const metric = deriveMetric${i}(state);`);
    L.push(`  return { section: ${i}, label, metric };`);
    L.push('}');
    L.push('');
  }
  L.push('// ---------------------------------------------------------------- entrypoint');
  L.push('export function renderPanel(state) {');
  L.push('  const sections = [');
  for (let i = 1; i <= 18; i++) L.push(`    renderSection${i}(state),`);
  L.push('  ];');
  L.push('  return {');
  L.push('    status: statusLabel(state?.status),');
  L.push('    sections,');
  L.push('    total: sections.reduce((a, s) => a + s.metric, 0),');
  L.push('  };');
  L.push('}');
  L.push('');
  while (L.length < 620) L.push('// padding to keep this file over the 500-line split threshold');
  w('src/big-panel.mjs', L.join('\n') + '\n');
}

// ------------------------------------------------------------------ tests

w('test/contract.test.mjs', `
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { render, plugin, ingest } from '../src/index.mjs';
import { statusLabel } from '../src/status-a.mjs';
import { knownPlugins } from '../src/registry.mjs';

test('renderPanel keeps its shape', () => {
  const out = render({ status: 'active', fields: {}, metrics: {} });
  assert.equal(out.status, 'Active');
  assert.equal(out.sections.length, 18);
  assert.equal(out.total, 0);
});

test('status labels are stable', () => {
  assert.equal(statusLabel('pending'), 'Pending review');
  assert.equal(statusLabel('nope'), 'Unknown');
});

test('plugin resolves through the string registry', async () => {
  assert.deepEqual(knownPlugins(), ['plugin-x']);
  const mod = await plugin('plugin-x');
  assert.equal(mod.id, 'plugin-x');
  assert.deepEqual(mod.run('ab'), { id: 'plugin-x', value: 'AB' });
  assert.equal(await plugin('missing'), null);
});

test('v1 and v2 payloads both read', () => {
  assert.deepEqual(ingest({ user_name: 'kim' }), { userName: 'kim', version: 1 });
  assert.deepEqual(ingest({ userName: 'lee' }), { userName: 'lee', version: 2 });
});
`);

w('AGENTS.md', `
# rl-fixture

Synthetic repository used to exercise refactor-me end to end.

Validation commands: \`npm run lint\`, \`npm run typecheck\`, \`npm test\`, \`npm run build\`.
\`npm run typecheck\` fails by design on \`src/broken.mjs\`.
`);
fs.copyFileSync(path.join(dest, 'AGENTS.md'), path.join(dest, 'CLAUDE.md'));

// ------------------------------------------------------------------ areas
//
// Two extra areas with genuinely different toolchains. Each plants one dead
// module of its own, so a run has something to find outside the root.

if (argv.includes('--multi')) {
  // --- app/web : bun
  w('app/web/package.json', `
{
  "name": "fixture-web",
  "private": true,
  "type": "module",
  "scripts": {
    "lint": "node tools/lint.mjs",
    "typecheck": "node tools/typecheck.mjs",
    "test": "bun test",
    "build": "node tools/build.mjs"
  }
}
`);
  fs.writeFileSync(path.join(dest, 'app/web/bun.lock'), '');   // makes bun the resolved runner

  w('app/web/tools/lint.mjs', `
import fs from 'node:fs';
import path from 'node:path';
const root = path.resolve(import.meta.dirname, '..', 'src');
let bad = 0;
for (const f of fs.readdirSync(root)) {
  if (!f.endsWith('.mjs')) continue;
  fs.readFileSync(path.join(root, f), 'utf8').split('\\n').forEach((line, i) => {
    if (/^\\s*var\\s/.test(line)) { console.error(\`src/\${f}:\${i + 1}: error L0001: unexpected \\\`var\\\`\`); bad++; }
  });
}
console.log(bad === 0 ? 'lint: clean' : \`lint: \${bad} problem(s)\`);
process.exit(bad === 0 ? 0 : 1);
`);
  w('app/web/tools/typecheck.mjs', `
console.log('typecheck: clean');
`);
  w('app/web/tools/build.mjs', `
import fs from 'node:fs';
import path from 'node:path';
const root = path.resolve(import.meta.dirname, '..');
const mod = await import(path.join(root, 'src', 'panel.mjs'));
if (typeof mod.renderPanel !== 'function') { console.error('build: error B0001: missing export'); process.exit(1); }
fs.mkdirSync(path.join(root, 'dist'), { recursive: true });
fs.writeFileSync(path.join(root, 'dist', 'manifest.json'), JSON.stringify({ exports: Object.keys(mod).sort() }));
console.log('build: ok');
`);
  w('app/web/src/panel.mjs', `
import { labelFor } from './labels.mjs';

export function renderPanel(state) {
  return { label: labelFor(state?.status), items: (state?.items ?? []).length };
}
`);
  w('app/web/src/labels.mjs', `
const LABELS = { active: 'Active', closed: 'Closed' };

export function labelFor(key) {
  return LABELS[key] ?? 'Unknown';
}
`);
  // Dead in app/web: nothing imports it, no string names it.
  w('app/web/src/legacy-formatter.mjs', `
// Replaced by labels.mjs. No import and no string reference remains.
export function formatLegacy(value) {
  return String(value).padEnd(12, ' ');
}

export function legacyWidth() {
  return 12;
}
`);
  w('app/web/panel.test.mjs', `
import { test, expect } from 'bun:test';
import { renderPanel } from './src/panel.mjs';

test('renderPanel keeps its shape', () => {
  expect(renderPanel({ status: 'active', items: [1, 2] })).toEqual({ label: 'Active', items: 2 });
  expect(renderPanel({}).label).toBe('Unknown');
});
`);
  w('app/web/.gitignore', 'node_modules/\ndist/\n');

  // --- app/api : Go. Module path is deliberately NOT a VCS path, so the
  // external-surface check resolves to NONE_DETECTED for this area too.
  // Real Go projects ignore their build output. Without this, `go build` leaves
  // a Mach-O binary named after the module and it lands in the change set.
  w('app/api/.gitignore', 'fixtureapi\n');

  w('app/api/go.mod', `
module fixtureapi

go 1.22
`);
  w('app/api/status.go', `
package main

import "fmt"

// StatusLabel is reached from main.
func StatusLabel(key string) string {
	switch key {
	case "active":
		return "Active"
	case "closed":
		return "Closed"
	}
	return "Unknown"
}

func main() {
	fmt.Println(StatusLabel("active"))
}
`);
  // Dead in app/api: unexported, unreferenced.
  w('app/api/legacy_parse.go', `
package main

import "strings"

// legacyTokenize was superseded and is referenced from nowhere.
func legacyTokenize(in string) []string {
	return strings.Fields(in)
}

func legacyVersion() int {
	return 1
}
`);
  w('app/api/status_test.go', `
package main

import "testing"

func TestStatusLabel(t *testing.T) {
	if got := StatusLabel("active"); got != "Active" {
		t.Fatalf("got %q", got)
	}
	if got := StatusLabel("nope"); got != "Unknown" {
		t.Fatalf("got %q", got)
	}
}
`);
  console.log('  multi-area: app/web (bun), app/api (go) — each with one dead module');
}

// ------------------------------------------------------------------ skills
//
// Mirror the convention the real targets use: .agents/skills holds the real
// directories (codex reads these), .claude/skills holds relative symlinks into
// them (claude reads these). Sourced from --skills <dir> so the fixture matches
// whatever catalog the operator actually has installed.

const skillsSrc = argv.includes('--skills') ? path.resolve(argv[argv.indexOf('--skills') + 1]) : null;
if (skillsSrc && fs.existsSync(skillsSrc)) {
  const agents = path.join(dest, '.agents', 'skills');
  const claude = path.join(dest, '.claude', 'skills');
  fs.mkdirSync(agents, { recursive: true });
  fs.mkdirSync(claude, { recursive: true });
  let n = 0;
  for (const e of fs.readdirSync(skillsSrc, { withFileTypes: true })) {
    const from = path.join(skillsSrc, e.name);
    if (!fs.existsSync(path.join(from, 'SKILL.md'))) continue;
    fs.cpSync(from, path.join(agents, e.name), { recursive: true, dereference: true });
    fs.symlinkSync(path.join('..', '..', '.agents', 'skills', e.name), path.join(claude, e.name));
    n++;
  }
  console.log(`  skills: ${n} copied from ${skillsSrc}`);
}

// ------------------------------------------------------------------ git

sh(['git', 'init', '-q', '-b', 'main']);
sh(['git', 'config', 'user.name', 'fixture']);
sh(['git', 'config', 'user.email', 'fixture@local']);
sh(['git', 'add', '-A']);
sh(['git', '-c', 'commit.gpgsign=false', 'commit', '-q', '-m', 'chore: fixture baseline']);

const head = sh(['git', 'rev-parse', 'HEAD']).trim();
console.log(`fixture ready: ${dest}`);
console.log(`  HEAD ${head.slice(0, 7)} on main`);
console.log(`  big-panel.mjs ${fs.readFileSync(path.join(dest, 'src/big-panel.mjs'), 'utf8').split('\n').length - 1} lines`);
console.log('  planted: legacy-parser (dead), plugin-x (TRAP), status-{a,b,c} (dup), big-panel (split), compat (v1 shim), broken (RED)');
