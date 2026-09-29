# Node 테스트 계약의 Go 대응표

> **2026-09-29 보관 기준:** 이 문서는 Homebrew 전환 전 조사·검증 기록입니다. 본문의 프로젝트별 설치, Node rollback, 과거 JSON 호환과 Skill 커밋 요건은 `0.10.0-beta.1`에 적용되지 않습니다. 현재 절차는 [Homebrew·전역 Skill 안내](README.ko.md), 명령·형식 계약은 [현재 참조 문서](../tool/README.ko.md)를 따르세요. 아래 테스트명과 실행 결과는 당시 근거이며 현재 테스트 목록이나 새 릴리스의 검증 결과가 아닙니다.

## 기준과 읽는 방법

2026-09-29 삭제 전 Node 테스트를 실행하여 **257개 통과**를 확인했다. 기준 소스는 [`fdf05a1`](https://github.com/soom-kang/refactor-me/tree/fdf05a1a2609ef15a185479c379a4a46eeb349b4/tool/test)이다. 아래 ID는 해당 실행의 TAP 순서이며, 반복 사례도 각각 기록했다.

- **이미 검증됨**: 기존 Go 테스트가 해당 계약을 검증한다.
- **Go 테스트 추가**: 이번 변경에서 Go 회귀 테스트를 추가했다. 서로 관련된 Node 사례는 Go table test로 합쳤다.
- **Node 전용으로 폐기**: Node 파일 배치나 이전 구현에만 존재하는 표시 규칙이다. 기존 Go 구현과의 차이를 비고에 적었다. 같은 동작이라고 주장하지 않는다. 이 차이는 이번 삭제에서 새로 도입한 정책이 아니라 이미 존재하던 Go 구현의 동작이다.

`package/TestName`은 `tool/go/internal/<package>`의 테스트다. 테스트명은 바로 검색할 수 있도록 그대로 기록했다. 개수의 일치가 아니라 입력·실패·보존 조건이 기준이다. 실제 provider 호출, 과금, 원격 게시 검증은 포함하지 않는다. JS/TS 대상 프로젝트 지원은 유지하며 이 대응표의 기본 테스트는 Node를 실행하지 않는다.

## 집계

| 분류 | Node 사례 수 |
|---|---:|
| 이미 검증됨 | 18 |
| Go 테스트 추가 | 229 |
| Node 전용으로 폐기 | 10 |

## 개별 계약

### audit-prompt.test.mjs

| ID | 삭제 전 Node 계약 | 처리 | Go 검증 / 변경된 의미 |
|---:|---|---|---|
| 1 | a re-audit with no commits does not claim the tree contains any | Go 테스트 추가 | `engine/TestMigrationAuditHistory` |
| 2 | a re-audit after commits still says the tree contains them | Go 테스트 추가 | `engine/TestMigrationAuditHistory` |
| 3 | a first-cycle audit carries no history block at all | Go 테스트 추가 | `engine/TestMigrationAuditHistory` |
| 4 | only real violations reach the forbidden-ground line | Go 테스트 추가 | `engine/TestMigrationAuditHistory` |
| 5 | each cycle gets its own audit artifact directory | Go 테스트 추가 | `workspace/TestMigrationAuditDirectoriesAndSelfIgnore` |

### catalog.test.mjs

| ID | 삭제 전 Node 계약 | 처리 | Go 검증 / 변경된 의미 |
|---:|---|---|---|
| 6 | installed catalog, lockfile, Claude links and routing agree on eight skills | Go 테스트 추가 | `engine/TestMigrationSkillCatalog` |
| 7 | new worktrees use the renamed cache and explicit parents still take precedence | Go 테스트 추가 | `workspace/TestMigrationNetSizeRules` |

### cli.test.mjs

| ID | 삭제 전 Node 계약 | 처리 | Go 검증 / 변경된 의미 |
|---:|---|---|---|
| 8 | help and version work outside a repository under the new name | 이미 검증됨 | `surface/TestParseAndVersionOutsideRepo` |
| 9 | invalid language arguments fail before creating any runtime files | 이미 검증됨 | `surface/TestParseAndVersionOutsideRepo` |
| 10 | report renders either language from JSON without rewriting stored evidence | Go 테스트 추가 | `surface/TestReportViewsPreserveFiles` |
| 11 | report with missing JSON fails without replacing existing Markdown | Go 테스트 추가 | `surface/TestMissingReportPreservesMarkdown` |
| 12 | install and reinstall provide the new command and preserve data and custom files | 이미 검증됨 | `surface/TestInstallPreservesDataAndRejectsUnknownFiles` |
| 13 | an incomplete distribution fails before replacing an existing installation | Go 테스트 추가 | `surface/TestInstallFailurePreservesPrevious` — Node 소스 묶음 검사 대신 단일 실행 파일 설치 실패 시 기존 명령 보존을 검증 |
| 14 | installer refuses an unrecognized old command before replacing files | 이미 검증됨 | `surface/TestInstallPreservesDataAndRejectsUnknownFiles` |

### code-comparison.test.mjs

| ID | 삭제 전 Node 계약 | 처리 | Go 검증 / 변경된 의미 |
|---:|---|---|---|
| 15 | net published comparison covers add, delete, rename, mode, binary and successive commits | Go 테스트 추가 | `controller/TestComparisonMigrationContracts` |
| 16 | collection works after a detached worktree is removed and includes characterization alone | Go 테스트 추가 | `controller/TestComparisonAfterWorktreeRemoval` |
| 17 | no published commit and net-zero published commits explicitly report no changes | Go 테스트 추가 | `controller/TestComparisonMigrationContracts + TestComparisonLimitsAndFailures` |
| 18 | preview respects complete lines and UTF-8 bytes: line limit | Go 테스트 추가 | `controller/TestComparisonLimitsAndFailures` |
| 19 | preview respects complete lines and UTF-8 bytes: byte limit | Go 테스트 추가 | `controller/TestComparisonLimitsAndFailures` |
| 20 | preview respects complete lines and UTF-8 bytes: one oversized line | Go 테스트 추가 | `controller/TestComparisonLimitsAndFailures` |
| 21 | embedded Markdown fences do not escape the diff block; context is three lines | Go 테스트 추가 | `surface/TestReportViewsPreserveFiles` — fence 길이와 파일명 escaping; patch context는 Go 구현 검사 |
| 22 | Git external diff and textconv are disabled | Go 테스트 추가 | `controller/TestComparisonMigrationContracts` |
| 23 | missing objects, non-OID refs and patch storage failures become unavailable | Go 테스트 추가 | `controller/TestComparisonLimitsAndFailures` |
| 24 | old JSON renders a missing-comparison notice | Go 테스트 추가 | `surface/TestReportMigrationEvidence` |
| 25 | CLI language views preserve saved JSON, Markdown and patch after repository changes | Go 테스트 추가 | `surface/TestReportViewsPreserveFiles` |

### config.test.mjs

| ID | 삭제 전 Node 계약 | 처리 | Go 검증 / 변경된 의미 |
|---:|---|---|---|
| 26 | the shipped defaults let the per-phase effort table decide | Go 테스트 추가 | `surface/TestConfigSampleMatchesInstalledDefaults` |
| 27 | the config file we install carries no blanket effort either | Go 테스트 추가 | `surface/TestConfigSampleMatchesInstalledDefaults` |
| 28 | a user who sets effort still overrides the table | 이미 검증됨 | `surface/TestLoadConfigMergesOldSettings` |
| 29 | an installation that predates unknown_risk still sets UNKNOWN aside | 이미 검증됨 | `surface/TestLoadConfigMergesOldSettings` |

### doctor.test.mjs

| ID | 삭제 전 Node 계약 | 처리 | Go 검증 / 변경된 의미 |
|---:|---|---|---|
| 30 | a report with one healthy provider is not blocked | Go 테스트 추가 | `controller/TestDoctorRenderingMigration` |
| 31 | no healthy provider is blocked | Go 테스트 추가 | `controller/TestDoctorRenderingMigration` |
| 32 | a clean report names no exclusions | Go 테스트 추가 | `controller/TestDoctorRenderingMigration` |
| 33 | a complete catalog passes even when the model under-reports it | Go 테스트 추가 | `controller/TestDoctorLiveFixtureProviderIsolation` — Go의 committed catalog와 실제 session 노출 기준으로 검증 |
| 34 | a complete catalog the session cannot see at all is a failure, not a warning | Go 테스트 추가 | `controller/TestDoctorLiveFixtureProviderIsolation` — Go의 committed catalog와 실제 session 노출 기준으로 검증 |
| 35 | a missing skill is named, and does not exclude a provider that can still see the rest | Node 전용으로 폐기 | — — Node helper는 누락 Skill을 WARN으로 허용했다. 기존 Go doctor는 committed required Skill 누락을 차단한다. TestDoctorCommittedSkillsAndCleanup으로 현재 보수적 계약을 검증 |
| 36 | nothing resolving and nothing visible is a plain uninstalled catalog | Go 테스트 추가 | `controller/TestDoctorLiveFixtureProviderIsolation` — Go의 committed catalog와 실제 session 노출 기준으로 검증 |
| 37 | the probed roots are reported with the home directory abbreviated | Node 전용으로 폐기 | — — Node probeRoots의 축약 경로 출력 helper는 Go에 없음. 경로·Skill 가시성은 doctor 검사로 표시 |
| 38 | claude probes no home directory, codex still does | Go 테스트 추가 | `controller/TestDoctorSkillRoots` — Claude project-only 및 Codex project/home 탐색 위치를 검증 |
| 39 | a repo-local skill that is not committed is reported, a committed one is not | Go 테스트 추가 | `controller/TestDoctorCommittedSkillsAndCleanup` — Go의 provider별 Skill 탐색과 commit 포함 여부/정리 계약 |
| 40 | a catalog reached through a committed .claude symlink is present in the checkout | Go 테스트 추가 | `controller/TestDoctorCommittedSkillsAndCleanup` — Go의 provider별 Skill 탐색과 commit 포함 여부/정리 계약 |
| 41 | a committed .claude symlink whose target is not committed is reported once | Go 테스트 추가 | `controller/TestDoctorCommittedSkillsAndCleanup` — Go의 provider별 Skill 탐색과 commit 포함 여부/정리 계약 |
| 42 | with no probe worktree the check reports that it did not measure | Go 테스트 추가 | `controller/TestDoctorProbeFailure` — probe worktree 생성 실패에서 결과를 조작하지 않고 실패를 표시 |
| 43 | doctor removes its probe worktree even when no provider CLI is installed | Go 테스트 추가 | `controller/TestDoctorCommittedSkillsAndCleanup` — Go의 provider별 Skill 탐색과 commit 포함 여부/정리 계약 |

### gate.test.mjs

| ID | 삭제 전 Node 계약 | 처리 | Go 검증 / 변경된 의미 |
|---:|---|---|---|
| 44 | glob: **/ matches at any depth including root | Go 테스트 추가 | `workspace/TestMigrationGateContracts` |
| 45 | glob: bare pattern matches the basename at any depth | Go 테스트 추가 | `workspace/TestMigrationGateContracts` |
| 46 | glob: * does not cross a path separator | Go 테스트 추가 | `workspace/TestMigrationGateContracts` |
| 47 | forbidden list covers every lockfile and oracle we care about | Go 테스트 추가 | `workspace/TestMigrationGateContracts` |
| 48 | test path detection spans js/go/python conventions | Go 테스트 추가 | `workspace/TestMigrationGateContracts` |
| 49 | weakening tokens are recognised across languages | Go 테스트 추가 | `workspace/TestMigrationGateContracts` |
| 50 | net size: a removal must actually remove | Go 테스트 추가 | `workspace/TestMigrationNetSizeRules` |
| 51 | net size: consolidation may not grow the codebase | Go 테스트 추가 | `workspace/TestMigrationNetSizeRules` |
| 52 | net size: a split redistributes, and must reach its stated goal | Go 테스트 추가 | `workspace/TestMigrationNetSizeRules` |
| 53 | clean candidate passes every controller check | Go 테스트 추가 | `workspace/TestMigrationGateContracts` |
| 54 | a moved worktree HEAD halts rather than merely failing | Go 테스트 추가 | `workspace/TestMigrationGateContracts` |
| 55 | a mutated source repository halts | Go 테스트 추가 | `workspace/TestMigrationGateContracts` |
| 56 | an empty change set is NO_OP, not a violation | Go 테스트 추가 | `workspace/TestMigrationGateContracts` |
| 57 | first failure wins: forbidden path outranks the allowlist | Go 테스트 추가 | `workspace/TestMigrationGateContracts` |
| 58 | binary content is rejected regardless of extension | Go 테스트 추가 | `workspace/TestMigrationGateContracts` |
| 59 | a path outside the allowlist is a violation | Go 테스트 추가 | `workspace/TestMigrationGateContracts` |
| 60 | a split may add siblings beside its allowlisted file | Go 테스트 추가 | `workspace/TestMigrationWorktreeSplitAndRollback` |
| 61 | deleting a test file is caught even inside the allowlist | Go 테스트 추가 | `workspace/TestMigrationNetSizeRules` |
| 62 | adding a skip marker to a test is caught | Go 테스트 추가 | `workspace/TestMigrationGateContracts` |
| 63 | a tree seen earlier in the run is thrash | Go 테스트 추가 | `workspace/TestMigrationGateContracts` |
| 64 | size caps fire before the category rule | Go 테스트 추가 | `workspace/TestMigrationGateContracts` |
| 65 | a split gets a line budget derived from the file it is splitting | Go 테스트 추가 | `workspace/TestMigrationNetSizeRules` |
| 66 | the preamble bounds "observable" so dead code can be removed at all | Go 테스트 추가 | `engine/TestMigrationPromptSafety` |
| 67 | phase routing restricts skills and reserves git ownership for the loop | Go 테스트 추가 | `engine/TestMigrationPromptSafety` |
| 68 | doctor probes exactly the skills routing can reach | Go 테스트 추가 | `engine/TestMigrationSkillCatalog` |
| 69 | declared deletions are checked against the allowlist like any other change | Go 테스트 추가 | `engine/TestMigrationVerdictEnforcers` |
| 70 | the execute prompt tells the model how to delete instead of letting it give up | Go 테스트 추가 | `engine/TestMigrationPromptSafety` |
| 71 | write prompts state that build output is not a boundary violation | Go 테스트 추가 | `engine/TestMigrationPromptSafety` |
| 72 | the check count matches the checks a clean run actually emits | Go 테스트 추가 | `workspace/TestMigrationGateContracts` |
| 73 | a change that lands entirely outside the target is rejected | Go 테스트 추가 | `workspace/TestMigrationGateContracts` |
| 74 | one changed path inside the target carries the rest | Go 테스트 추가 | `workspace/TestMigrationNetSizeRules` |
| 75 | an unscoped run passes the target step without restricting anything | Go 테스트 추가 | `workspace/TestMigrationGateContracts` |

### jvm.test.mjs

| ID | 삭제 전 Node 계약 | 처리 | Go 검증 / 변경된 의미 |
|---:|---|---|---|
| 76 | a gradle build with a JVM plugin gets a compile check and a test run | Go 테스트 추가 | `workspace/TestMigrationDiscovery` |
| 77 | a Kotlin JVM build in kts is recognised too | Go 테스트 추가 | `workspace/TestMigrationDiscovery` |
| 78 | a gradle build with no JVM plugin contributes nothing at all | Go 테스트 추가 | `workspace/TestMigrationDiscovery` |
| 79 | a plugin applied only in a subproject still counts | Go 테스트 추가 | `workspace/TestMigrationDiscovery` |
| 80 | the gradle wrapper is used only when it is actually executable | Go 테스트 추가 | `workspace/TestMigrationJVMWrapperAndAreas` |
| 81 | maven needs no plugin evidence, because its phases are fixed by the tool | Go 테스트 추가 | `workspace/TestMigrationDiscovery` |
| 82 | a nested JVM module is not an area of its own | Go 테스트 추가 | `workspace/TestMigrationJVMWrapperAndAreas` |
| 83 | a JS front end beside a gradle build keeps both test commands | Go 테스트 추가 | `workspace/TestMigrationJVMWrapperAndAreas` |
| 84 | a relative wrapper resolves against the area, not the process cwd | Go 테스트 추가 | `workspace/TestMigrationJVMWrapperAndAreas` |
| 85 | JVM failures produce a signature at all | Go 테스트 추가 | `workspace/TestMigrationValidationSignatures` |
| 86 | build-level status lines are deliberately not signatures | Go 테스트 추가 | `workspace/TestMigrationValidationSignatures` |
| 87 | a test count is not a signature, because adding a test would trip it | Go 테스트 추가 | `workspace/TestMigrationValidationSignatures` |
| 88 | surefire elapsed time never enters the signature | Go 테스트 추가 | `workspace/TestMigrationValidationSignatures` |
| 89 | a Maven column change is tolerated the way a line shift is | Go 테스트 추가 | `workspace/TestMigrationValidationSignatures` |
| 90 | a failure nobody can read is excluded rather than trusted | Go 테스트 추가 | `workspace/TestMigrationDifferentialLadder` |
| 91 | JVM manifests cannot be edited by the agent | Go 테스트 추가 | `workspace/TestMigrationGateContracts` |
| 92 | ordinary Java sources are still editable | Go 테스트 추가 | `workspace/TestMigrationGateContracts` |
| 93 | JVM golden resources are protected like every other oracle | Go 테스트 추가 | `workspace/TestMigrationGateContracts` |
| 94 | JVM test paths are recognised as tests | Go 테스트 추가 | `workspace/TestMigrationGateContracts` |
| 95 | disabling a JUnit test counts as weakening it | Go 테스트 추가 | `workspace/TestMigrationGateContracts` |
| 96 | a build root at the repository root is not outranked by anything outside it | Go 테스트 추가 | `workspace/TestMigrationJVMWrapperAndAreas` |
| 97 | a check that stops being readable fails the ladder instead of passing it | Go 테스트 추가 | `workspace/TestMigrationDifferentialLadder` |
| 98 | a legacy apply-plugin line names the plugin exactly | Go 테스트 추가 | `workspace/TestMigrationDiscovery` |

### provider.test.mjs

| ID | 삭제 전 Node 계약 | 처리 | Go 검증 / 변경된 의미 |
|---:|---|---|---|
| 99 | claude read mode has no write tool and no Bash | 이미 검증됨 | `engine/TestProviderArguments` |
| 100 | claude write mode grants Edit/Write but still no Bash | 이미 검증됨 | `engine/TestProviderArguments` |
| 101 | claude never receives the flags that would disable skills or break auth | Go 테스트 추가 | `engine/TestMigrationPromptSafety` |
| 102 | codex passes exactly two -c overrides | 이미 검증됨 | `engine/TestProviderArguments` |
| 103 | codex sets reasoning effort even without a model, because --ignore-user-config resets it | Go 테스트 추가 | `engine/TestMigrationPromptSafety` |
| 104 | codex sandbox follows the mode and never skips the git check | Go 테스트 추가 | `engine/TestMigrationPromptSafety` |
| 105 | lastJsonObject survives a leading banner and braces inside strings | Go 테스트 추가 | `engine/TestMigrationProviderParsing` |
| 106 | stripFences removes a json code fence | Go 테스트 추가 | `engine/TestMigrationProviderParsing` |
| 107 | claude success is is_error === false, never subtype | Go 테스트 추가 | `engine/TestMigrationProviderParsing` |
| 108 | claude quota is distinguished from a transient rate limit | Go 테스트 추가 | `engine/TestMigrationProviderParsing` |
| 109 | claude schema exhaustion is SCHEMA, not PROCESS | Go 테스트 추가 | `engine/TestMigrationProviderParsing` |
| 110 | claude success without structured output is SCHEMA | Go 테스트 추가 | `engine/TestMigrationProviderParsing` |
| 111 | a benign codex JSONL error event on an exit-0 run is NOT a failure | 이미 검증됨 | `engine/TestTaxonomy` |
| 112 | a codex schema-file rejection is fatal and must not be repaired or failed over | 이미 검증됨 | `engine/TestTaxonomy` |
| 113 | codex quota and auth are recognised from stderr | Go 테스트 추가 | `engine/TestMigrationProviderParsing` |
| 114 | a timeout is classified without parsing anything | Go 테스트 추가 | `engine/TestMigrationProviderParsing` |
| 115 | a hard quota puts a provider in cooldown and hands over to the other | Go 테스트 추가 | `engine/TestMigrationProviderRing` |
| 116 | a soft quota does not remove the provider from the ring | Go 테스트 추가 | `engine/TestMigrationProviderRing` |
| 117 | an auth failure kills the provider permanently for this run | Go 테스트 추가 | `engine/TestMigrationProviderRing` |
| 118 | an expired cooldown returns the provider to the ring | Go 테스트 추가 | `engine/TestMigrationProviderRing` |
| 119 | review prefers the provider that did not write, and degrades gracefully | Go 테스트 추가 | `engine/TestMigrationProviderRing` |
| 120 | only the two write phases may write; everything else fails closed | Go 테스트 추가 | `engine/TestMigrationAuditHistory` |
| 121 | a request that claims write for a non-write phase is refused, not granted | 이미 검증됨 | `engine/TestWritePrivilegeFailsClosed` |
| 122 | an unstated mode defaults to read rather than write | Go 테스트 추가 | `engine/TestMigrationAuditHistory` |
| 123 | effort is chosen per phase, and config can override it | Go 테스트 추가 | `engine/TestMigrationAuditHistory` |
| 124 | every claude phase can actually invoke a skill | Go 테스트 추가 | `engine/TestMigrationPromptSafety` |
| 125 | the operator's provider order is honoured, not the array order | Go 테스트 추가 | `engine/TestMigrationProviderRing` |
| 126 | a single-provider order never returns the excluded one | Go 테스트 추가 | `engine/TestMigrationProviderRing` |

### rank.test.mjs

| ID | 삭제 전 Node 계약 | 처리 | Go 검증 / 변경된 의미 |
|---:|---|---|---|
| 127 | L3 is never executed, whatever the model says | Go 테스트 추가 | `controller/TestMigrationRankContracts` |
| 128 | an unknown risk level is set aside for a human, not executed | Go 테스트 추가 | `controller/TestMigrationRankContracts` |
| 129 | UNKNOWN cannot be enabled by widening allowed_risks | Go 테스트 추가 | `controller/TestMigrationRankContracts` |
| 130 | a candidate the model itself rejected is not retried | Go 테스트 추가 | `controller/TestMigrationRankContracts` |
| 131 | an oversized candidate is filtered before it can be planned | Go 테스트 추가 | `controller/TestMigrationRankContracts` |
| 132 | completed and abandoned work is never re-proposed, even reworded | Go 테스트 추가 | `controller/TestMigrationRankSeenAndOrder` |
| 133 | a candidate stopped by provider exhaustion stays eligible | Go 테스트 추가 | `controller/TestMigrationRankSeenAndOrder` |
| 134 | a candidate is abandoned once its attempts are spent | Go 테스트 추가 | `controller/TestMigrationRankSeenAndOrder` |
| 135 | ranking prefers lower risk, then readiness, then a smaller blast radius | Go 테스트 추가 | `controller/TestMigrationRankSeenAndOrder` |
| 136 | a skipped candidate records the paths a later audit is warned about | Go 테스트 추가 | `controller/TestMigrationPolicyAndHistory` |
| 137 | commit subjects do not double the verb | Node 전용으로 폐기 | — — Node commitSubject helper의 자동 동사·언어별 가공은 Go에 없음. Go는 packet title 또는 candidate_id를 사용 |
| 138 | a non-Latin title is used verbatim rather than given an English verb | Node 전용으로 폐기 | — — Node commitSubject helper의 자동 동사·언어별 가공은 Go에 없음. Go는 packet title 또는 candidate_id를 사용 |
| 139 | preflight log lines lead with the verdict, not the objection | Node 전용으로 폐기 | — — Node 전용 preflight log 문구 정규식 검사. Go는 phase state와 verdict JSON을 기록 |
| 140 | the handoff brief declares itself a mid-run snapshot in either language | Go 테스트 추가 | `controller/TestMigrationHandoffSnapshot` — Go header는 두 언어 공통 English이며 최종 보고서가 아닌 snapshot임을 검증 |
| 141 | a candidate with no file inside the target is set aside | Go 테스트 추가 | `controller/TestMigrationRankContracts` |
| 142 | one file inside the target is enough, however many lie outside | Go 테스트 추가 | `controller/TestMigrationRankContracts` |
| 143 | an unscoped run filters nothing on scope grounds | Go 테스트 추가 | `controller/TestMigrationRankContracts` |
| 144 | the target filter does not match a sibling with a longer name | Go 테스트 추가 | `controller/TestMigrationRankContracts` |
| 145 | the shipped default sets an unknown risk aside | 이미 검증됨 | `surface/TestLoadConfigMergesOldSettings` |
| 146 | an UNKNOWN candidate that names its missing check reaches deep check when the policy says so | Go 테스트 추가 | `controller/TestMigrationRankContracts` |
| 147 | a passed-through UNKNOWN does not fall into the allowed_risks test | Go 테스트 추가 | `controller/TestMigrationRankContracts` |
| 148 | deep_check passes through exactly one shape | Go 테스트 추가 | `controller/TestMigrationRankContracts` |
| 149 | a stated risk outside the policy is still excluded under deep_check | Go 테스트 추가 | `controller/TestMigrationRankContracts` |
| 150 | a passed-through UNKNOWN never outranks a stated risk level | Go 테스트 추가 | `controller/TestMigrationRankSeenAndOrder` |
| 151 | an invalid unknown_risk stops the run before it starts | Go 테스트 추가 | `controller/TestMigrationPolicyAndHistory` |
| 152 | the packet risk gate reads the packet, not the audit | Go 테스트 추가 | `controller/TestMigrationPacketRiskGate` |
| 153 | unknown_risk never relaxes the gate that runs after deep check | Go 테스트 추가 | `controller/TestMigrationPacketRiskGate` |
| 154 | a candidate filtered during ranking does not forbid its paths | Go 테스트 추가 | `controller/TestMigrationPolicyAndHistory` |
| 155 | a path an attempt actually violated is forbidden | Go 테스트 추가 | `controller/TestMigrationPolicyAndHistory` |
| 156 | a skip record written before the flag existed forbids nothing | Go 테스트 추가 | `controller/TestMigrationPolicyAndHistory` |
| 157 | the RISK_UNKNOWN record carries the audit own objection, not a skill name | Go 테스트 추가 | `controller/TestMigrationRankContracts` |
| 158 | a cycle where everything was filtered says so | Node 전용으로 폐기 | — — Node rank log의 집계 문자열 검사. Go는 auditsAllFiltered counter와 report reason으로 기록 |
| 159 | the stop reason names which kind of empty audit ended the run | Go 테스트 추가 | `controller/TestMigrationPolicyAndHistory` |

### repo-facts.test.mjs

| ID | 삭제 전 Node 계약 | 처리 | Go 검증 / 변경된 의미 |
|---:|---|---|---|
| 160 | an unscoped run sees every area | Go 테스트 추가 | `engine/TestMigrationScopedInventory` |
| 161 | a scoped run lists only the target | Go 테스트 추가 | `engine/TestMigrationScopedInventory` |
| 162 | a scoped run says the unlisted files are still reachable | Go 테스트 추가 | `engine/TestMigrationScopedInventory` |
| 163 | multiple targets are all in scope | Go 테스트 추가 | `engine/TestMigrationScopedInventory` |
| 164 | the large-file statistic follows the scope | Go 테스트 추가 | `engine/TestMigrationScopedInventory` |

### report.test.mjs

| ID | 삭제 전 Node 계약 | 처리 | Go 검증 / 변경된 의미 |
|---:|---|---|---|
| 165 | the provider arrow follows the order the run actually used | Node 전용으로 폐기 | — — Node compact renderSummary의 provider 화살표/생략 규칙. Go는 provider usage table을 표시 |
| 166 | a provider that was never called is omitted | Node 전용으로 폐기 | — — Node compact renderSummary의 provider 화살표/생략 규칙. Go는 provider usage table을 표시 |
| 167 | an older state without providerOrder still renders | Go 테스트 추가 | `surface/TestReportMigrationEvidence` |
| 168 | a codex-only run is never priced at zero dollars | Go 테스트 추가 | `surface/TestReportMigrationEvidence` |
| 169 | a mixed run reports a floor, not a total | Go 테스트 추가 | `surface/TestReportMigrationEvidence` |
| 170 | a fully priced run just says the number | Go 테스트 추가 | `surface/TestReportMigrationEvidence` |
| 171 | a run that called no provider gets no cost section at all | Go 테스트 추가 | `surface/TestReportMigrationEvidence` |
| 172 | a report written before usage existed still renders | Go 테스트 추가 | `surface/TestReportMigrationEvidence` |
| 173 | the summary leads with the build that produced the run | Node 전용으로 폐기 | — — Node compact summary의 첫 줄 레이아웃. Go report build field와 기록 보존으로 대체 |
| 174 | a run from before versioning still renders, without leaking undefined | Go 테스트 추가 | `surface/TestReportMigrationEvidence` |
| 175 | both languages preserve evidence for DONE | Go 테스트 추가 | `surface/TestReportMigrationEvidence` |
| 176 | both languages preserve evidence for NO_CHANGES | Go 테스트 추가 | `surface/TestReportMigrationEvidence` |
| 177 | both languages preserve evidence for DONE_PARTIAL | Go 테스트 추가 | `surface/TestReportMigrationEvidence` |
| 178 | both languages preserve evidence for ABORTED | Go 테스트 추가 | `surface/TestReportMigrationEvidence` |
| 179 | both languages preserve evidence for HALTED_UNSAFE | Go 테스트 추가 | `surface/TestReportMigrationEvidence` |
| 180 | Korean reports retain unknown and partial cost semantics | Go 테스트 추가 | `surface/TestReportMigrationEvidence` |
| 181 | buildReport writes one selected Markdown language and language-neutral JSON | Go 테스트 추가 | `controller/TestReportGenerationLanguageNeutral` |
| 182 | Korean terminal reasons translate known limits while preserving raw details | Go 테스트 추가 | `surface/TestReportReasonsAndEmptyAuditMigration` |
| 183 | the report says which kind of empty audit ended the run | Go 테스트 추가 | `surface/TestReportReasonsAndEmptyAuditMigration` |
| 184 | a report written before those counters existed renders neither the line nor undefined | Go 테스트 추가 | `surface/TestReportReasonsAndEmptyAuditMigration` |

### scope.test.mjs

| ID | 삭제 전 Node 계약 | 처리 | Go 검증 / 변경된 의미 |
|---:|---|---|---|
| 185 | containment respects the path separator | Go 테스트 추가 | `workspace/TestMigrationScopeBoundaries` |
| 186 | an empty target list restricts nothing | Go 테스트 추가 | `workspace/TestMigrationScopeBoundaries` |
| 187 | a candidate needs only one related file inside the scope | Go 테스트 추가 | `workspace/TestMigrationScopeBoundaries` |
| 188 | targets resolve against the directory the operator typed them in | 이미 검증됨 | `workspace/TestScopeAndSignatures` |
| 189 | a trailing slash and a duplicate collapse to one target | Go 테스트 추가 | `workspace/TestMigrationScopeBoundaries` |
| 190 | a nested target collapses into its parent | Go 테스트 추가 | `workspace/TestMigrationScopeBoundaries` |
| 191 | a target outside the repository is refused | Go 테스트 추가 | `workspace/TestMigrationScopeBoundaries` |
| 192 | a missing directory and a file are both refused | Go 테스트 추가 | `workspace/TestMigrationScopeBoundaries` |
| 193 | the repository root is refused rather than treated as a scope | Go 테스트 추가 | `workspace/TestMigrationScopeBoundaries` |
| 194 | source detection skips vendored trees | Go 테스트 추가 | `workspace/TestMigrationScopeBoundaries` |

### usage.test.mjs

| ID | 삭제 전 Node 계약 | 처리 | Go 검증 / 변경된 의미 |
|---:|---|---|---|
| 195 | an unreported cost stays null instead of collapsing to zero | Go 테스트 추가 | `engine/TestMigrationUsageAccounting` |
| 196 | a partial cost is kept, and how much is missing is kept with it | Go 테스트 추가 | `engine/TestMigrationUsageAccounting` |
| 197 | costOf never prices an unreported total at zero | Go 테스트 추가 | `engine/TestMigrationUsageAccounting` |
| 198 | noteUsage fills the provider ring and the phase bucket at once | Go 테스트 추가 | `engine/TestMigrationUsageAccounting` |
| 199 | a failed call still counts toward what the run spent | Go 테스트 추가 | `engine/TestMigrationUsageAccounting` |
| 200 | a repaired call is one call but two processes | Go 테스트 추가 | `engine/TestMigrationUsageAccounting` |
| 201 | a fresh accumulator reports nothing rather than zero dollars | Go 테스트 추가 | `engine/TestMigrationUsageAccounting` |
| 202 | claude usage is read off the result envelope, cost included | Go 테스트 추가 | `engine/TestMigrationUsageAccounting` |
| 203 | claude without a reported cost yields null, not zero | Go 테스트 추가 | `engine/TestMigrationUsageAccounting` |
| 204 | token counts read at a glance and durations read as clock time | Go 테스트 추가 | `engine/TestMigrationUsageAccounting` |
| 205 | codex reports tokens but never a cost, and that null is the contract | Go 테스트 추가 | `engine/TestMigrationUsageAccounting` |
| 206 | claude input tokens include the cached ones, so both providers mean the same thing | Go 테스트 추가 | `engine/TestMigrationUsageAccounting` |

### validate.test.mjs

| ID | 삭제 전 Node 계약 | 처리 | Go 검증 / 변경된 의미 |
|---:|---|---|---|
| 207 | signature extraction picks up file:line and error codes | Go 테스트 추가 | `workspace/TestMigrationValidationSignatures` |
| 208 | signature extraction handles go and node test failures | Go 테스트 추가 | `workspace/TestMigrationValidationSignatures` |
| 209 | a line-number shift in an already-failing file is tolerated | Go 테스트 추가 | `workspace/TestMigrationValidationSignatures` |
| 210 | a complaint in a NEW file is a regression | Go 테스트 추가 | `workspace/TestMigrationValidationSignatures` |
| 211 | a new error code is a regression even in a known file | Go 테스트 추가 | `workspace/TestMigrationValidationSignatures` |
| 212 | fixing something is never a failure | Go 테스트 추가 | `workspace/TestMigrationValidationSignatures` |
| 213 | area scoping picks the deepest matching area | Go 테스트 추가 | `workspace/TestMigrationDifferentialLadder` |
| 214 | a heavy test script is recorded as skipped, never executed | Go 테스트 추가 | `workspace/TestMigrationDiscovery` |
| 215 | the package runner follows the lockfile | Go 테스트 추가 | `workspace/TestMigrationDiscovery` |
| 216 | go areas never get -race or integration tags | Go 테스트 추가 | `workspace/TestMigrationDiscovery` |
| 217 | a Makefile test target survives a pyproject that only contributes lint | Go 테스트 추가 | `workspace/TestMigrationDiscovery` |
| 218 | a python tool that is only configured gets the bare binary, a declared one gets the runner | Go 테스트 추가 | `workspace/TestMigrationDiscovery` |
| 219 | a binary that is not installed is UNRUNNABLE, never a RED differential gate | Go 테스트 추가 | `workspace/TestMigrationDifferentialLadder` |
| 220 | an unrunnable baseline command is dropped from the ladder instead of passing it | Go 테스트 추가 | `workspace/TestMigrationDifferentialLadder` |
| 221 | a baseline RED check that goes unreadable is caught, not waved through | Go 테스트 추가 | `workspace/TestMigrationDifferentialLadder` |
| 222 | the fixture discovers all four commands in one area | Go 테스트 추가 | `workspace/TestMigrationDiscovery` — 임시 JS manifest의 4개 명령 구조를 검증하며 Node 실행 불필요 |
| 223 | baseline is usable despite a permanently RED typecheck | Go 테스트 추가 | `workspace/TestMigrationIgnoredHydrationAndBaseline` |
| 224 | the ladder passes when the RED command fails identically | Go 테스트 추가 | `workspace/TestMigrationDifferentialLadder` |
| 225 | the ladder fails when a NEW file starts complaining | Go 테스트 추가 | `workspace/TestMigrationDifferentialLadder` |
| 226 | the ladder fails when a GREEN command turns RED | Go 테스트 추가 | `workspace/TestMigrationDifferentialLadder` |
| 227 | a private package with no entrypoints reports NONE_DETECTED | Go 테스트 추가 | `engine/TestMigrationExternalSurface` |
| 228 | a published entrypoint or release workflow reports POSSIBLE | Go 테스트 추가 | `engine/TestMigrationExternalSurface` |
| 229 | an importable go module path counts as an external surface | Go 테스트 추가 | `engine/TestMigrationExternalSurface` |

### version.test.mjs

| ID | 삭제 전 Node 계약 | 처리 | Go 검증 / 변경된 의미 |
|---:|---|---|---|
| 230 | development builds identify themselves as dev | 이미 검증됨 | `surface/TestParseAndVersionOutsideRepo` |
| 231 | the changelog records the next planned release | 이미 검증됨 | `CI independent release version check` — tool/RELEASE_VERSION·CHANGELOG·binary version의 독립 대조 |
| 232 | every run records which build produced it | Go 테스트 추가 | `controller/TestMigrationPolicyAndHistory` |
| 233 | the version file is inside src/, where install copies from | Node 전용으로 폐기 | — — Node src/version.mjs 복사 위치 계약은 단일 Go 실행 파일 배포에서 사라짐 |

### workflow.test.mjs

| ID | 삭제 전 Node 계약 | 처리 | Go 검증 / 변경된 의미 |
|---:|---|---|---|
| 234 | Workflow editions share full JSON examples that match the actual phase schemas | Go 테스트 추가 | `engine/TestMigrationWorkflowExamples` |
| 235 | Workflow success and rejection responses follow the current verdict enforcers | Go 테스트 추가 | `engine/TestMigrationWorkflowExamples` |
| 236 | audit UNKNOWN is filtered while NEEDS_EVIDENCE can reach deep check | Go 테스트 추가 | `controller/TestMigrationRankContracts` |
| 237 | validation follows the documented affected-area scope and rejects a new regression | Go 테스트 추가 | `workspace/TestMigrationAffectedAreaLadder` |

### worktree-gate.test.mjs

| ID | 삭제 전 Node 계약 | 처리 | Go 검증 / 변경된 의미 |
|---:|---|---|---|
| 238 | the worktree is a real, detached checkout of the base commit | Go 테스트 추가 | `workspace/TestMigrationWorktreeArtifacts` |
| 239 | a fresh worktree presents an empty change set | Go 테스트 추가 | `workspace/TestMigrationWorktreeArtifacts` |
| 240 | an in-scope deletion passes the gate | Go 테스트 추가 | `workspace/TestMigrationWorktreeSplitAndRollback` |
| 241 | an out-of-scope edit is caught at the allowlist check and rolled back exactly | Go 테스트 추가 | `workspace/TestMigrationRealGateRefusals` |
| 242 | touching a lockfile is FORBIDDEN even when the allowlist names it | Go 테스트 추가 | `workspace/TestMigrationRealGateRefusals` |
| 243 | rollback preserves hydrated gitignored build inputs | Go 테스트 추가 | `workspace/TestMigrationIgnoredHydrationAndBaseline` |
| 244 | the run directory self-ignores, so writing into it cannot dirty the repo | Go 테스트 추가 | `workspace/TestMigrationAuditDirectoriesAndSelfIgnore` |
| 245 | the source repository is untouched by everything above | Go 테스트 추가 | `workspace/TestMigrationWorktreeArtifacts` |
| 246 | a commit in the worktree is publishable into the source by update-ref alone | 이미 검증됨 | `controller/TestRunPublishesReviewedDeletion` |
| 247 | a split that creates new files is measured on its true net size | Go 테스트 추가 | `workspace/TestMigrationWorktreeSplitAndRollback` |
| 248 | a split that rewrites rather than redistributes is still rejected | Go 테스트 추가 | `workspace/TestMigrationNetSizeRules` |
| 249 | a declared deletion inside the allowlist is applied by the orchestrator | 이미 검증됨 | `controller/TestRunPublishesReviewedDeletion` |
| 250 | a declared deletion outside the allowlist is refused and deletes nothing | Go 테스트 추가 | `controller/TestMigrationDeclaredDeletionRefusal` |
| 251 | weakening a real test file is caught by the real gate | Go 테스트 추가 | `workspace/TestMigrationRealGateRefusals` |
| 252 | mutating the source repository mid-candidate halts rather than committing | Go 테스트 추가 | `workspace/TestMigrationRealGateRefusals` |
| 253 | a compiled artifact left by a validation command is swept, not blamed on the agent | Go 테스트 추가 | `workspace/TestMigrationWorktreeArtifacts` |
| 254 | a JVM build report is swept only inside the build output it came from | Go 테스트 추가 | `workspace/TestMigrationWorktreeArtifacts` |
| 255 | the sweep leaves gitignored and empty files alone | Go 테스트 추가 | `workspace/TestMigrationIgnoredHydrationAndBaseline` |
| 256 | the review diff names untracked binaries but never dumps their bytes | Go 테스트 추가 | `workspace/TestMigrationWorktreeArtifacts` |
| 257 | sweeping twice is harmless, because the ladder recreates artifacts | Go 테스트 추가 | `workspace/TestMigrationWorktreeArtifacts` |

## 재검증

```sh
cd tool/go
go test -race -count=1 ./...
go vet ./...
```

CI는 Node 실행을 거부하는 PATH guard를 앞에 두고 같은 Go suite를 실행한다. JavaScript fixture의 실제 실행은 별도 선택 검증이며, 기본 계약 테스트는 임시 manifest와 기록된 JSON을 사용한다.
