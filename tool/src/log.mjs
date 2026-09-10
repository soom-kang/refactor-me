// log.mjs — three sinks, strictly separated.
//   stderr : live human stream, never JSON, ANSI only on a TTY
//   stdout : final summary only (or one JSON object under --json)
//   files  : events.ndjson (machine truth) + console.log (verbatim stderr copy)

import fs from 'node:fs';
import path from 'node:path';

const MARK = {
  start: '[>]', pass: '[+]', fail: '[x]', warn: '[!]', info: '[.]', retry: '[~]',
};

const COLOR = {
  '[>]': '\x1b[36m', '[+]': '\x1b[32m', '[x]': '\x1b[31m',
  '[!]': '\x1b[33m', '[.]': '\x1b[90m', '[~]': '\x1b[35m',
};
const RESET = '\x1b[0m';

const oneLine = (s, max = 160) => {
  const flat = String(s).replace(/\s+/g, ' ').trim();
  return flat.length > max ? `${flat.slice(0, max - 1)}…` : flat;
};

function elapsed(startedAt) {
  const s = Math.floor((Date.now() - startedAt) / 1000);
  const mm = String(Math.floor(s / 60)).padStart(2, '0');
  const ss = String(s % 60).padStart(2, '0');
  return `${mm}:${ss}`;
}

export class Logger {
  /**
   * @param {{runDir?:string, color?:boolean, quiet?:boolean}} opts
   */
  constructor(opts = {}) {
    this.startedAt = Date.now();
    this.cycle = null;             // number | null  -> "c01" / "---"
    this.state = 'INIT';
    this.runDir = opts.runDir ?? null;
    this.color = opts.color ?? (process.stderr.isTTY === true);
    this.quiet = opts.quiet ?? false;
    this._events = null;           // fd for events.ndjson
    this._console = null;          // fd for console.log
    this._lastLineAt = Date.now();
    this._heartbeat = null;
  }

  /** Late-bind the run directory once it exists (INIT creates it). */
  attach(runDir) {
    this.runDir = runDir;
    fs.mkdirSync(runDir, { recursive: true });
    this._events = fs.openSync(path.join(runDir, 'events.ndjson'), 'a');
    this._console = fs.openSync(path.join(runDir, 'console.log'), 'a');
  }

  setCycle(n) { this.cycle = n; }
  setState(s) { this.state = s; }

  _prefix() {
    const c = this.cycle == null ? '---' : `c${String(this.cycle).padStart(2, '0')}`;
    return `${elapsed(this.startedAt)}  ${c} ${this.state.padEnd(12)}`;
  }

  /**
   * Emit one live line to stderr + console.log, and one structured record to
   * events.ndjson. `mark` is a key of MARK.
   */
  line(mark, message, data) {
    const m = MARK[mark] ?? MARK.info;
    // One event is one line. Tool invocations and error text routinely contain
    // newlines; letting them through turns the live stream into something that
    // cannot be scanned or grepped.
    const plain = `${this._prefix()} ${m} ${oneLine(message)}`;
    this._lastLineAt = Date.now();

    if (!this.quiet) {
      const painted = this.color ? `${COLOR[m] ?? ''}${plain}${RESET}` : plain;
      process.stderr.write(painted + '\n');
    }
    if (this._console !== null) fs.writeSync(this._console, plain + '\n');
    this.event(mark, message, data);
  }

  /** Structured record only — no terminal output. */
  event(mark, message, data) {
    if (this._events === null) return;
    const rec = {
      t: new Date().toISOString(),
      ms: Date.now() - this.startedAt,
      cycle: this.cycle,
      state: this.state,
      mark,
      message,
      ...(data === undefined ? {} : { data }),
    };
    fs.writeSync(this._events, JSON.stringify(rec) + '\n');
  }

  start(msg, d) { this.line('start', msg, d); }
  pass(msg, d) { this.line('pass', msg, d); }
  fail(msg, d) { this.line('fail', msg, d); }
  warn(msg, d) { this.line('warn', msg, d); }
  info(msg, d) { this.line('info', msg, d); }
  retry(msg, d) { this.line('retry', msg, d); }

  /**
   * While a long provider call runs, emit "still working" every `everyMs` of
   * silence. `describe()` returns the suffix, e.g. "34 tool calls".
   */
  beginHeartbeat(describe, everyMs = 15000) {
    this.endHeartbeat();
    const began = Date.now();
    this._heartbeat = setInterval(() => {
      if (Date.now() - this._lastLineAt < everyMs) return;
      const secs = Math.round((Date.now() - began) / 1000);
      const mins = Math.floor(secs / 60);
      const span = mins > 0 ? `${mins}m${String(secs % 60).padStart(2, '0')}s` : `${secs}s`;
      const extra = describe?.() ?? '';
      this.info(`still working (${span}${extra ? ', ' + extra : ''})`);
    }, Math.min(everyMs, 5000));
    this._heartbeat.unref?.();
  }

  endHeartbeat() {
    if (this._heartbeat) { clearInterval(this._heartbeat); this._heartbeat = null; }
  }

  /** Final summary goes to stdout so `| jq` and `> log` both stay sane. */
  summary(text) { process.stdout.write(text.endsWith('\n') ? text : text + '\n'); }
  json(obj) { process.stdout.write(JSON.stringify(obj, null, 2) + '\n'); }

  close() {
    this.endHeartbeat();
    if (this._events !== null) { fs.closeSync(this._events); this._events = null; }
    if (this._console !== null) { fs.closeSync(this._console); this._console = null; }
  }
}
