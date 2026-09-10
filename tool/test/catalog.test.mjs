import { test } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { REQUIRED_SKILLS, ROUTING } from '../src/prompts.mjs';
import { worktreePath } from '../src/git.mjs';
import os from 'node:os';

const root = path.resolve(import.meta.dirname, '../..');
test('installed catalog, lockfile, Claude links and routing agree on eight skills', () => {
  const expected = ['sharpen-clarify', 'sharpen-review', 'sharpen-challenge', 'sharpen-assess',
    'sharpen-refine', 'sharpen-cold-review', 'sharpen-brief', 'sharpen-dedupe'];
  assert.deepEqual([...REQUIRED_SKILLS].sort(), [...expected].sort());
  const lock = JSON.parse(fs.readFileSync(path.join(root, 'skills-lock.json'), 'utf8'));
  assert.deepEqual(Object.keys(lock.skills).sort(), [...expected].sort());
  for (const name of expected) {
    const skill = path.join(root, '.agents', 'skills', name);
    const md = fs.readFileSync(path.join(skill, 'SKILL.md'), 'utf8');
    assert.match(md, new RegExp(`^name: ${name}$`, 'm'));
    assert.equal(fs.realpathSync(path.join(root, '.claude', 'skills', name)), fs.realpathSync(skill));
    assert.equal(lock.skills[name].source, 'soom-kang/sharpen-me');
    assert.equal(lock.skills[name].skillPath, `skills/${name}/SKILL.md`);
  }
  assert.deepEqual(ROUTING.EXECUTE, ['sharpen-refine']);
  assert.deepEqual(ROUTING.REVIEW, ['sharpen-cold-review']);
});

test('new worktrees use the renamed cache and explicit parents still take precedence', () => {
  assert.ok(worktreePath('/repo', 'run').startsWith(path.join(os.homedir(), '.cache', 'refactor-me') + path.sep));
  assert.ok(worktreePath('/repo', 'run', '/custom').startsWith('/custom/'));
});
