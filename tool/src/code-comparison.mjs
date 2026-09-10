import fs from 'node:fs';
import path from 'node:path';
import crypto from 'node:crypto';
import { spawnSync } from 'node:child_process';
import { git } from './git.mjs';

const PREVIEW_BYTES = 32 * 1024;
const PREVIEW_LINES = 200;

function previewOf(file) {
  const fd = fs.openSync(file, 'r');
  try {
    const size = fs.fstatSync(fd).size;
    const buffer = Buffer.alloc(PREVIEW_BYTES);
    const read = fs.readSync(fd, buffer, 0, buffer.length, 0);
    let end = 0;
    let lines = 0;
    for (let i = 0; i < read; i++) {
      if (buffer[i] === 10) {
        end = i + 1;
        if (++lines === PREVIEW_LINES) break;
      }
    }
    if (read === size && lines < PREVIEW_LINES) end = read;
    return { preview: buffer.subarray(0, end).toString('utf8'), truncated: end < size };
  } finally {
    fs.closeSync(fd);
  }
}

function filesOf(repoRoot, args) {
  const raw = git(repoRoot, [...args, '--raw', '-z', '--']).split('\0');
  const files = [];
  for (let i = 0; i < raw.length && raw[i];) {
    const [oldMode, newMode, , , status] = raw[i++].slice(1).split(' ');
    const firstPath = raw[i++];
    const renamed = /^[RC]/.test(status);
    files.push({ path: renamed ? raw[i++] : firstPath, oldPath: renamed ? firstPath : null,
      status, oldMode, newMode, insertions: 0, deletions: 0, binary: false });
  }
  const byPath = new Map(files.map((file) => [file.path, file]));
  const stats = git(repoRoot, [...args, '--numstat', '-z', '--']).split('\0');
  for (let i = 0; i < stats.length && stats[i];) {
    const match = stats[i++].match(/^([^\t]+)\t([^\t]+)\t([\s\S]*)$/);
    if (!match) throw new Error('Invalid Git numstat record');
    const [, added, removed, name] = match;
    let filePath = name;
    if (!filePath) { i++; filePath = stats[i++]; }
    const file = byPath.get(filePath);
    if (!file) throw new Error('Git metadata and numstat paths differ');
    file.binary = added === '-' || removed === '-';
    file.insertions = file.binary ? null : Number(added);
    file.deletions = file.binary ? null : Number(removed);
  }
  return files;
}

/** Collect only published Git objects. A reporting failure never changes run status. */
export function collectCodeComparison(state, runDir) {
  const result = {
    status: 'NO_CHANGES', baseCommit: state.baseOid ?? null, resultCommit: state.publishedOid ?? null,
    files: [], totals: { files: 0, insertions: 0, deletions: 0, binary: 0 },
    patchFile: null, preview: '', truncated: false, error: null,
  };
  // No published commit means no accepted changes, including on initialization failure.
  if (!state.publishedOid) return result;
  let temporary;
  try {
    for (const oid of [state.baseOid, state.publishedOid]) {
      if (!/^(?:[a-f0-9]{40}|[a-f0-9]{64})$/i.test(oid ?? '')) throw new Error('Comparison requires full commit OIDs');
      git(state.repoRoot, ['rev-parse', '--verify', '--end-of-options', `${oid}^{commit}`]);
    }
    const args = ['-c', 'core.quotePath=true', 'diff', '--no-ext-diff', '--no-textconv', '--no-color',
      '--find-renames=50%', '--diff-algorithm=myers', '--no-relative',
      '--src-prefix=a/', '--dst-prefix=b/', '--submodule=short', '--ignore-submodules=none', state.baseOid, state.publishedOid];
    const files = filesOf(state.repoRoot, args);
    const totals = { files: files.length, insertions: 0, deletions: 0, binary: 0 };
    for (const file of files) {
      totals.insertions += file.insertions ?? 0;
      totals.deletions += file.deletions ?? 0;
      totals.binary += Number(file.binary);
    }
    temporary = path.join(runDir, `.changes-${crypto.randomUUID()}.patch`);
    const fd = fs.openSync(temporary, 'wx', 0o600);
    try {
      const response = spawnSync('git', [...args, '--patch', '--unified=3', '--'], {
        cwd: state.repoRoot, stdio: ['ignore', fd, 'pipe'], encoding: 'utf8',
        env: { ...process.env, GIT_TERMINAL_PROMPT: '0', GIT_OPTIONAL_LOCKS: '0' },
      });
      if (response.error) throw response.error;
      if (response.status !== 0) throw new Error(`Git patch collection failed (${response.status}): ${response.stderr?.trim() ?? ''}`);
    } finally {
      fs.closeSync(fd);
    }
    const preview = previewOf(temporary);
    fs.renameSync(temporary, path.join(runDir, 'changes.patch'));
    return { ...result, status: files.length ? 'AVAILABLE' : 'NO_CHANGES', files, totals,
      patchFile: 'changes.patch', ...preview };
  } catch (error) {
    return { ...result, status: 'UNAVAILABLE', error: String(error.message ?? error) };
  } finally {
    if (temporary) { try { fs.unlinkSync(temporary); } catch { /* Already renamed or never created. */ } }
  }
}
