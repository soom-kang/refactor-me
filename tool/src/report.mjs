// Reports render the same JSON in English or Korean without translating evidence.
import fs from 'node:fs';
import path from 'node:path';
import * as S from './state.mjs';
import { mergeUsage, newUsage, costOf, fmtMs, fmtTokens, PHASE_EFFORT } from './provider.mjs';

export function validateLanguage(language) {
  if (!['en', 'ko'].includes(language)) throw new Error(`unsupported language: ${language}; use en or ko`);
  return language;
}
const translator = (language) => {
  validateLanguage(language);
  return (en, ko) => language === 'ko' ? ko : en;
};
const STATUS = {
  DONE: ['Completed', '완료'], NO_CHANGES: ['No changes', '변경 없음'],
  DONE_PARTIAL: ['Partially completed', '부분 완료'], ABORTED: ['Aborted', '중단'],
  HALTED_UNSAFE: ['Safety halt', '안전 정지'], UNKNOWN: ['Unknown', '알 수 없음'],
};
const statusLabel = (status, t) => STATUS[status] ? t(...STATUS[status]) : status;
const REASON_KO = {
  RISK_EXCLUDED: '허용 위험도 밖', MODEL_REJECTED: '감사 단계에서 거절', ATTEMPTS_EXHAUSTED: '시도 횟수 소진',
  RISK_UNKNOWN: '위험도 판정 근거 부족', OUT_OF_TARGET: '대상 폴더 밖',
  TARGET_SCOPE_EMPTY: '대상 폴더 밖만 수정',
  TOO_LARGE: '범위 초과', NOT_READY: '증거 부족', PREFLIGHT_BLOCKED: 'preflight 차단',
  NO_OP: '변경 없음', FORBIDDEN_PATH: '금지 경로 수정', FORBIDDEN_BINARY: '바이너리 변경',
  OUT_OF_SCOPE: 'allowlist 밖 수정', NET_SIZE_RULE: '증감 규칙 위반', TEST_WEAKENED: '테스트 약화',
  THRASH_REVERT: '이전 작업 되돌림', REGRESSION: '검증 회귀', VALIDATION_TIMEOUT: '검증 타임아웃',
  REVIEW_REJECT: '독립 리뷰 거절', EXECUTE_REJECTED: '구현 자체 중단', EXECUTE_FAILED: '구현 실패',
  DEEP_CHECK_FAILED: '심층 검증 실패', REVIEW_FAILED: '리뷰 실패',
  CHARACTERIZATION_REJECTED: 'characterization 범위 위반', CHARACTERIZATION_RED: 'characterization 테스트 실패',
};
const REASON_EN = {
  RISK_EXCLUDED: 'Risk outside policy', MODEL_REJECTED: 'Rejected during audit', ATTEMPTS_EXHAUSTED: 'Attempt limit reached',
  RISK_UNKNOWN: 'Insufficient evidence to assess risk', OUT_OF_TARGET: 'Outside target directories',
  TARGET_SCOPE_EMPTY: 'Changes only outside target directories', TOO_LARGE: 'Scope limit exceeded',
  NOT_READY: 'Insufficient evidence', PREFLIGHT_BLOCKED: 'Blocked at preflight', NO_OP: 'No changes',
  FORBIDDEN_PATH: 'Forbidden path changed', FORBIDDEN_BINARY: 'Binary changed', OUT_OF_SCOPE: 'Outside allowlist',
  NET_SIZE_RULE: 'Size rule violated', TEST_WEAKENED: 'Test weakened', THRASH_REVERT: 'Earlier work reversed',
  REGRESSION: 'Validation regression', VALIDATION_TIMEOUT: 'Validation timeout', REVIEW_REJECT: 'Rejected by independent review',
  EXECUTE_REJECTED: 'Implementation declined', EXECUTE_FAILED: 'Implementation failed',
  DEEP_CHECK_FAILED: 'Deep check failed', REVIEW_FAILED: 'Review failed',
  CHARACTERIZATION_REJECTED: 'Characterization scope violated', CHARACTERIZATION_RED: 'Characterization test failed',
};
const reasonLabel = (reason, t) => t(REASON_EN[reason] ?? reason, REASON_KO[reason] ?? reason);

// Only known controller messages are localized. Provider and error text stays intact.
function terminalReason(reason, t) {
  const known = {
    'no eligible candidates remain': '실행할 수 있는 후보가 남아 있지 않습니다',
    'doctor found a blocking problem': 'doctor가 실행을 막는 문제를 발견했습니다',
    'no deterministic validation command could be discovered': '반복 실행할 검증 명령을 찾지 못했습니다',
  };
  if (known[reason]) return t(reason, known[reason]);
  const patterns = [
    [/^stopped on cycle budget \((\d+)\)$/, (m) => `사이클 한도 ${m[1]}회에 도달했습니다`],
    [/^stopped on commit budget \((\d+)\)$/, (m) => `커밋 한도 ${m[1]}개에 도달했습니다`],
    [/^stopped on (\d+) consecutive failures$/, (m) => `후보가 ${m[1]}회 연속 실패했습니다`],
    [/^stopped on wall clock \((\d+(?:\.\d+)?)m\)$/, (m) => `경과 시간 한도 ${m[1]}분에 도달했습니다`],
    [/^stopped on all providers exhausted$/, () => '사용 가능한 프로바이더가 없습니다'],
    [/^no validation command could be executed: (.*)$/s, (m) => `검증 명령을 실행하지 못했습니다: ${m[1]}`],
    [/^every validation command failed at baseline; with no trustworthy signal there is no behavior-preservation evidence to be had(.*)$/s,
      (m) => `기준선의 검증 명령이 모두 실패해 동작 보존을 판단할 근거가 없습니다${m[1]}`],
  ];
  for (const [pattern, translate] of patterns) {
    const match = typeof reason === 'string' ? reason.match(pattern) : null;
    if (match) return t(reason, translate(match));
  }
  const code = typeof reason === 'string' ? reason.match(/^([A-Z_]+): (.*)$/s) : null;
  if (code && REASON_KO[code[1]]) return t(reason, `${reasonLabel(code[1], t)} (${code[1]}): ${code[2]}`);
  return reasonLabel(reason ?? '', t);
}

export function renderHandoffHeader(deadProvider, liveProvider, failureClass, language = 'en') {
  const t = translator(language);
  return [
    `<!-- generated mid-run at the ${failureClass} handoff; not the final outcome -->`,
    t(`> **Snapshot at the ${deadProvider} → ${liveProvider} handoff.**`,
      `> **이 문서는 ${deadProvider} → ${liveProvider} 전환 시점의 스냅샷입니다.**`),
    t('> Execution may have continued after this snapshot.', '> 실행은 이 시점 이후로도 계속되었을 수 있습니다.'),
    t('> Read `report.md` and `git log` for the final result.', '> 최종 결과는 `report.md`와 `git log`를 확인하세요.'),
    '', '---', '',
  ].join('\n');
}

const PHASE_ORDER = ['doctor', 'audit', 'deep_check', 'characterization', 'preflight', 'execute', 'review', 'handoff'];
function totalUsage(byPhase) {
  let total = { ...newUsage(), calls: 0 };
  for (const usage of Object.values(byPhase ?? {})) {
    total = { ...mergeUsage(total, usage), calls: (total.calls ?? 0) + (usage.calls ?? 0) };
  }
  return total;
}

function costSentence(usage, t) {
  const cost = costOf(usage);
  if (!cost.text) return t('- Cost: **Not reported**. No monetary total is available for these calls.',
    '- 비용: **미보고**. 이 호출의 비용 합계를 확인할 수 없습니다.');
  if (cost.partial) return t(`- Cost: **${cost.text} or more**. ${usage.costMissing} process(es) did not report cost.`,
    `- 비용: **${cost.text} 이상**. 프로세스 ${usage.costMissing}개가 비용을 보고하지 않았습니다.`);
  return t(`- Cost: **${cost.text}**`, `- 비용: **${cost.text}**`);
}

function renderUsage(j, t) {
  const total = j.usage?.totals;
  if (!total || total.processes === 0) return [];
  const byPhase = j.usage.byPhase ?? {};
  const phases = [...PHASE_ORDER.filter((p) => byPhase[p]),
    ...Object.keys(byPhase).filter((p) => !PHASE_ORDER.includes(p)).sort()];
  const lines = [t('## Cost and time', '## 비용과 시간'), '',
    t(`- Duration: ${j.durationMinutes} min (${j.providerMinutes} min in providers; the remainder includes validation and Git)`,
      `- 총 소요: ${j.durationMinutes}분 (프로바이더 ${j.providerMinutes}분, 나머지는 검증과 Git 작업 포함)`),
    t(`- Providers: ${total.calls} calls, ${total.processes} CLI processes, ${total.failedCalls ?? 0} failed calls`,
      `- 프로바이더: 호출 ${total.calls}회, CLI 프로세스 ${total.processes}개, 실패한 호출 ${total.failedCalls ?? 0}회`),
    t(`- Tokens: ${total.inputTokens.toLocaleString('en-US')} input, ${total.outputTokens.toLocaleString('en-US')} output`,
      `- 토큰: 입력 ${total.inputTokens.toLocaleString('en-US')}, 출력 ${total.outputTokens.toLocaleString('en-US')}`),
    costSentence(total, t), '',
    t('| Phase | Effort | Calls | Duration | Input | Output | Cost |', '| 단계 | 추론 수준 | 호출 | 소요 | 입력 | 출력 | 비용 |'),
    '|---|---|---|---|---|---|---|'];
  for (const phase of phases) {
    const usage = byPhase[phase];
    const cost = costOf(usage);
    const effort = phase === 'audit' && (usage.calls ?? 0) > 1 ? 'high→medium' : (PHASE_EFFORT[phase] ?? '-');
    lines.push(`| ${phase} | ${effort} | ${usage.calls ?? 0} | ${fmtMs(usage.ms)} | ${fmtTokens(usage.inputTokens)} | ${fmtTokens(usage.outputTokens)} | ${cost.text ? (cost.partial ? `${cost.text}+` : cost.text) : t('Not reported', '미보고')} |`);
  }
  lines.push('', t('Effort shows the default policy; provider configuration can override it. A plus sign marks a partial cost.',
    '추론 수준은 기본 정책입니다. 프로바이더 설정으로 바꿀 수 있습니다. 비용의 +는 일부 비용만 집계했음을 뜻합니다.'), '');
  return lines;
}
export function buildReport(ctx, language = 'en') {
  validateLanguage(language);
  const { state, baseline, commands, skippedCommands, hints, runDir } = ctx;
  const t = state.terminal ?? { status: 'UNKNOWN', reason: '' };
  const mins = Math.round((Date.now() - ctx.startedAt) / 60000);
  const usageTotals = totalUsage(state.usage?.byPhase);

  const json = {
    runId: state.runId,
    // Read from the state, not from VERSION: a report can be rebuilt later, and
    // the fact worth recording is which build did the run — not which build is
    // installed now.
    toolVersion: state.toolVersion ?? null,
    status: t.status,
    reason: t.reason,
    durationMinutes: mins,
    // Wall clock minus this is validation commands, git and prompt assembly —
    // the difference is the answer to "was it waiting on the model or on me".
    providerMinutes: Math.round(usageTotals.ms / 60000),
    usage: { totals: usageTotals, byPhase: state.usage?.byPhase ?? {} },
    repoRoot: state.repoRoot,
    targets: state.targets ?? [],
    baseCommit: state.baseOid,
    baseBranch: state.baseBranch,
    // The branch exists if anything at all was published to it — a
    // characterization commit counts, and a report that hides it sends the
    // operator looking for nothing.
    branch: state.publishedOid ? state.branchName : null,
    worktree: state.worktree,
    providers: state.providers,
    providerOrder: state.providerOrder ?? Object.keys(state.providers),
    counters: state.counters,
    commits: state.commits,
    skipped: state.seen.skipped,
    validation: {
      describe: baseline?.describe ?? null,
      ran: (commands ?? []).map((c) => `${c.area} ${c.tier} ${c.name}: ${c.argv.join(' ')}`),
      notRun: [
        ...(skippedCommands ?? []).map((c) => `${c.area} ${c.name}: ${c.argv.join(' ')} — ${c.reason}`),
        ...(hints ?? []).map((h) => `${h.file}: ${h.command} — CI only, never executed by the loop`),
      ],
    },
  };
  S.writeJsonAtomic(path.join(runDir, 'report.json'), json);
  fs.writeFileSync(path.join(runDir, 'report.md'), renderMarkdown(json, language));
  return json;
}

export function renderMarkdown(j, language = 'en') {
  const t = translator(language);
  const lines = [`# refactor-me ${j.runId}: ${statusLabel(j.status, t)}`, '',
    t(`- Duration: ${j.durationMinutes} min`, `- 소요: ${j.durationMinutes}분`),
    t(`- Tool version: refactor-me ${j.toolVersion ?? 'Not recorded'}`, `- 도구 버전: refactor-me ${j.toolVersion ?? '기록 없음'}`),
    t(`- Repository: \`${j.repoRoot}\``, `- 저장소: \`${j.repoRoot}\``),
    `${t('- Target directories', '- 대상 폴더')}: ${j.targets?.length ? j.targets.map((target) => `\`${target}\``).join(', ') : t('Entire repository', '저장소 전체')}`,
    `${t('- Base commit', '- 기준 커밋')}: \`${j.baseCommit.slice(0, 7)}\` (${j.baseBranch ?? 'detached'})`,
    j.branch
      ? t(`- Result branch: \`${j.branch}\` (${j.commits.length} commits: ${j.counters.commits} refactors, ${j.commits.length - j.counters.commits} characterization)`,
          `- 결과 브랜치: \`${j.branch}\` (커밋 ${j.commits.length}개: 리팩터링 ${j.counters.commits}개, characterization ${j.commits.length - j.counters.commits}개)`)
      : t('- Result branch: None (no committed changes)', '- 결과 브랜치: 없음 (커밋된 변경 없음)'),
    `${t('- Stop reason', '- 종료 사유')}: ${terminalReason(j.reason, t)}`, '',
    t('## Committed changes', '## 커밋된 변경'), ''];
  if (!j.commits.length) lines.push(t('None.', '없음.'));
  for (const commit of j.commits) {
    lines.push(`- \`${commit.oid.slice(0, 7)}\` **${commit.category}** ${commit.subject}`,
      `  - ${t('Paths', '경로')}: ${commit.paths.map((p) => `\`${p}\``).join(', ')}`);
  }
  lines.push('', t('## Skipped candidates', '## 제외한 후보'), '');
  if (!j.skipped.length) lines.push(t('None.', '없음.'));
  const grouped = new Map();
  for (const skipped of j.skipped) {
    if (!grouped.has(skipped.reason)) grouped.set(skipped.reason, []);
    grouped.get(skipped.reason).push(skipped);
  }
  for (const [reason, items] of [...grouped].sort((a, b) => b[1].length - a[1].length)) {
    lines.push(`- **${reasonLabel(reason, t)}** (\`${reason}\`) × ${items.length}`);
    for (const item of items.slice(0, 5)) {
      const detail = item.detail?.detail ?? item.detail;
      if (detail) lines.push(`  - ${String(typeof detail === 'string' ? detail : JSON.stringify(detail)).slice(0, 200)}`);
    }
  }
  lines.push('', t('## Validation', '## 검증'), '',
    `${t('Baseline', '기준선')}: ${j.validation.describe ?? t('Not recorded', '기록 없음')}`, '',
    t('For commands that failed at baseline, validation compares error signatures. Matching failures do not count as passing checks.',
      '기준선에서 실패한 명령은 오류 signature를 비교합니다. 같은 오류로 실패한 검사를 통과로 집계하지 않습니다.'), '',
    t('Commands run:', '실행한 명령:'), '');
  for (const command of j.validation.ran) lines.push(`- \`${command}\``);
  if (j.validation.notRun.length) {
    lines.push('', t('### Checks not run', '### 실행하지 않은 검사'), '',
      t('Review the recorded reasons and run the applicable checks before merging.', '기록된 사유를 확인하고 병합 전에 필요한 검사를 실행하세요.'), '');
    for (const command of j.validation.notRun) lines.push(`- \`${command}\``);
  }
  lines.push('', t('## Providers', '## 프로바이더'), '');
  for (const [name, provider] of Object.entries(j.providers)) {
    lines.push(`- **${name}**: ${provider.status} · ${provider.calls}${t(' calls', '회 호출')}`
      + (provider.quotaHits ? ` · ${t('quota hits', '쿼터 제한')} ${provider.quotaHits}` : '')
      + (provider.lastError ? ` · ${t('last error', '마지막 오류')}: ${String(provider.lastError).slice(0, 120)}` : ''));
  }
  lines.push('', ...renderUsage(j, t), t('## Next steps', '## 다음 단계'), '');
  if (j.branch) {
    lines.push('```bash', `git log --oneline ${j.baseCommit.slice(0, 7)}..${j.branch}`,
      `git diff --stat ${j.baseCommit.slice(0, 7)}...${j.branch}`, '```', '',
      t('Review the local branch before merging. refactor-me does not merge, push, or deploy.',
        '로컬 브랜치를 검토한 뒤 병합하세요. refactor-me는 merge, push, 배포를 수행하지 않습니다.'));
  } else lines.push(t('No changes were committed; there is no result branch to review.', '커밋된 변경이 없어 검토할 결과 브랜치가 없습니다.'));
  if (j.status === 'HALTED_UNSAFE') lines.push('',
    t('> **Safety halt.** An invariant failed and the loop stopped writing.', '> **안전 정지.** 불변식이 깨져 쓰기를 중단했습니다.'),
    t(`> Worktree retained as evidence: \`${j.worktree}\``, `> 증거로 보존한 worktree: \`${j.worktree}\``));
  return [...lines, ''].join('\n');
}

export function renderSummary(j, runDir, language = 'en') {
  const t = translator(language);
  const line = (key, value) => `  ${key.padEnd(11)}${value}`;
  const lines = [`refactor-me ${j.toolVersion ? `${j.toolVersion} · ` : ''}${statusLabel(j.status, t)} · ${j.durationMinutes}${t(' min', '분')}`];
  const order = (j.providerOrder ?? []).length ? j.providerOrder : Object.keys(j.providers);
  const ring = order.filter((name) => j.providers[name]?.calls > 0)
    .map((name) => `${name}(${j.providers[name].calls}${j.providers[name].status !== 'READY' ? ` ${j.providers[name].status}` : ''})`).join(' → ');
  lines.push(line(t('provider', '프로바이더'), ring || t('none', '없음')));
  if (j.usage?.totals?.processes) {
    const usage = j.usage.totals;
    const cost = costOf(usage);
    const costText = cost.text ? (cost.partial ? `${cost.text}+` : cost.text) : t('cost not reported', '비용 미보고');
    lines.push(line(t('usage', '사용량'), `${usage.calls} ${t('calls', '회 호출')} · ${j.providerMinutes}${t('m in provider', '분 프로바이더')} · ${fmtTokens(usage.inputTokens)}↓/${fmtTokens(usage.outputTokens)}↑ · ${costText}`));
  }
  const chars = j.commits.filter((commit) => commit.category === 'CHARACTERIZATION').length;
  lines.push(line(t('cycles', '실행'), t(`${j.counters.cycles} cycles · ${j.counters.commits} refactor + ${chars} characterization commit(s)`,
    `${j.counters.cycles}회 · 리팩터링 ${j.counters.commits}개 + characterization ${chars}개 커밋`)));
  lines.push(line(t('skipped', '제외'), `${j.skipped.length}  ${t('violations', '위반')} ${j.counters.violations}`));
  if (j.targets?.length) lines.push(line(t('target', '대상'), j.targets.join(', ')));
  lines.push(line(t('branch', '브랜치'), j.branch ?? t('(none; no commits)', '(없음, 커밋 없음)')),
    line(t('validated', '검증'), j.validation.describe ?? t('n/a', '기록 없음')));
  if (j.validation.notRun.length) lines.push(line(t('not run', '미실행'), `${j.validation.notRun.length} ${t('command(s); see report', '개 명령, 리포트 참고')}`));
  lines.push(line(t('report', '리포트'), path.join(runDir, 'report.md')), line('worktree', j.worktree));
  if (j.branch) lines.push(line(t('inspect', '확인'), `git log --oneline ${j.baseCommit.slice(0, 7)}..${j.branch}`));
  return lines.join('\n');
}
