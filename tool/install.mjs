#!/usr/bin/env node
// install.mjs — copy refactor-me into a target repository.
//
// Install is a directory copy plus a .gitignore containing "*". That single
// file makes .refactor/ invisible to git status, which is why no ownership
// manifest, no SHA-256 inventory and no uninstall bookkeeping are needed:
// nothing we write can be committed by accident, so there is nothing to track.
//
//   node install.mjs <target-repo>        install or update
//   node install.mjs <target-repo> --uninstall

import fs from 'node:fs';
import path from 'node:path';
import { spawnSync } from 'node:child_process';
import { VERSION } from './src/version.mjs';

const here = import.meta.dirname;
const [targetArg, ...flags] = process.argv.slice(2);

if (!targetArg) {
  console.error('usage: node install.mjs <target-repo> [--uninstall]');
  process.exit(2);
}
const target = path.resolve(targetArg);

const root = spawnSync('git', ['-C', target, 'rev-parse', '--show-toplevel'], { encoding: 'utf8' });
if (root.status !== 0) {
  console.error(`${target} is not a git repository.`);
  console.error('refactor-me publishes its result as a local branch, so git is not optional.');
  process.exit(2);
}
const repo = fs.realpathSync(root.stdout.trim());
const dest = path.join(repo, '.refactor');

// Remove only the exact shim shape written by the previous installer.
// An operator-created file or symlink with this name is not ours to delete.
const previousShim = path.join(dest, 'bin', 'refactorloop');
let removePreviousShim = false;
if (fs.lstatSync(previousShim, { throwIfNoEntry: false })) {
  if (fs.lstatSync(previousShim).isFile()) {
    const content = fs.readFileSync(previousShim, 'utf8');
    const lines = content.split('\n');
    removePreviousShim = lines.length === 4 && lines[0] === '#!/bin/sh'
      && /^# refactorloop \d+\.\d+\.\d+ — installed \d{4}-\d{2}-\d{2} from .+$/.test(lines[1])
      && lines[2] === 'exec node "$(dirname "$0")/../lib/bin/refactorloop.mjs" "$@"' && lines[3] === '';
  }
  if (!removePreviousShim && !flags.includes('--uninstall')) {
    console.error(`cannot replace installation: unrecognized file at ${previousShim}`);
    process.exit(2);
  }
}

if (flags.includes('--uninstall')) {
  // Only our own files. runs/ is evidence and config.json is the operator's.
  for (const p of ['lib', 'bin']) fs.rmSync(path.join(dest, p), { recursive: true, force: true });
  console.log(`removed ${dest}/lib and ${dest}/bin`);
  console.log(`kept    ${dest}/runs, ${dest}/config.json (evidence and your settings)`);
  process.exit(0);
}

fs.mkdirSync(path.join(dest, 'lib'), { recursive: true });
fs.mkdirSync(path.join(dest, 'bin'), { recursive: true });
fs.writeFileSync(path.join(dest, '.gitignore'), '*\n');

fs.rmSync(path.join(dest, 'lib', 'src'), { recursive: true, force: true });
fs.rmSync(path.join(dest, 'lib', 'bin'), { recursive: true, force: true });
fs.cpSync(path.join(here, 'src'), path.join(dest, 'lib', 'src'), { recursive: true });
fs.cpSync(path.join(here, 'bin'), path.join(dest, 'lib', 'bin'), { recursive: true });

// The comment line is provenance, not configuration: nothing reads it back.
// `head -2 .refactor/bin/refactor-me` answers "which build is this and where
// did it come from" without running anything — the shim is rewritten on every
// install, so it cannot go stale.
const shim = path.join(dest, 'bin', 'refactor-me');
fs.writeFileSync(shim, [
  '#!/bin/sh',
  `# refactor-me ${VERSION} — installed ${new Date().toISOString().slice(0, 10)} from ${here}`,
  'exec node "$(dirname "$0")/../lib/bin/refactor-me.mjs" "$@"',
  '',
].join('\n'));
fs.chmodSync(shim, 0o755);
if (removePreviousShim) fs.unlinkSync(previousShim);

const cfg = path.join(dest, 'config.json');
if (!fs.existsSync(cfg)) {
  fs.copyFileSync(path.join(here, 'config.default.json'), cfg);
  console.log(`created ${path.relative(repo, cfg)} (defaults; edit if you want)`);
}

const status = spawnSync('git', ['-C', repo, 'status', '--porcelain'], { encoding: 'utf8' }).stdout.trim();
console.log(`installed refactor-me ${VERSION} into ${dest}`);
console.log(status === ''
  ? '  the working tree is still clean — .refactor/ ignores itself'
  : `  note: the tree has pre-existing changes:\n${status.split('\n').slice(0, 5).map((l) => '    ' + l).join('\n')}`);
console.log('');
console.log('next:');
console.log(`  cd ${repo}`);
console.log('  ./.refactor/bin/refactor-me doctor      # check preconditions');
console.log('  ./.refactor/bin/refactor-me             # run the loop');
