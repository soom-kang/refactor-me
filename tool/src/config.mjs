// The configuration a run actually uses: our defaults, merged with whatever
// .refactor/config.json supplies.
//
// This lives here rather than in bin/refactor-me.mjs because that file calls
// main() at import time — anything defined there is unreachable from a test.
// The merge below is exactly where a stray setting would slip back in, so it
// has to be reachable.
//
// Deliberately absent: agents.<name>.effort. effortFor() resolves
// `effort_by_phase[phase] ?? effort ?? PHASE_EFFORT[phase]`, so shipping a
// blanket `effort` here would shadow the per-phase table in provider.mjs
// entirely and run every phase at `high` — including preflight and
// characterization, which is the spend that table exists to avoid. A user may
// still set it; we must not ship it.

import path from 'node:path';
import { readJson } from './state.mjs';
import { DEFAULT_POLICY } from './loop.mjs';

export const DEFAULT_CONFIG = {
  // Written to every config.json and never read back — loadConfig below merges
  // regardless of its value, and there is no migration. It is a placeholder for
  // a scheme that does not exist yet, not a compatibility check. What actually
  // identifies a build is VERSION in version.mjs.
  schema_version: 1,
  workspace: { branch_prefix: 'refactor/auto-', worktree_parent: '', keep_worktree: true },
  agents: {
    primary: 'codex',
    fallback: 'claude',
    codex: { enabled: true, bin: 'codex', model: '', timeout_sec: 1800 },
    claude: { enabled: true, bin: 'claude', model: '', timeout_sec: 1800, max_budget_usd: 0 },
  },
  policy: DEFAULT_POLICY,
  verification: { locked: false, commands: [] },
};

export function loadConfig(repoRoot) {
  const f = path.join(repoRoot, '.refactor', 'config.json');
  const user = readJson(f, null);
  if (!user) return structuredClone(DEFAULT_CONFIG);
  return {
    ...DEFAULT_CONFIG, ...user,
    workspace: { ...DEFAULT_CONFIG.workspace, ...(user.workspace ?? {}) },
    agents: {
      ...DEFAULT_CONFIG.agents, ...(user.agents ?? {}),
      codex: { ...DEFAULT_CONFIG.agents.codex, ...(user.agents?.codex ?? {}) },
      claude: { ...DEFAULT_CONFIG.agents.claude, ...(user.agents?.claude ?? {}) },
    },
    policy: { ...DEFAULT_POLICY, ...(user.policy ?? {}) },
    verification: { ...DEFAULT_CONFIG.verification, ...(user.verification ?? {}) },
  };
}
