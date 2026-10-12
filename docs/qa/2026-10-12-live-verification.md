# QA follow-up verification — 2026-10-12

[QA 수정 기록](2026-10-12-remediation.md) · [티켓 12건](2026-10-12-tickets.md)

**합의한 provider 연결, Homebrew candidate 설치·업그레이드, vulnerability scan을 모두 검증했습니다.** 대상 source는 QA 수정 commit `f0055f1713544482f5d63bc3d30b0d79491d3bea`입니다. 시작 시 checkout은 clean이었고, 실행을 마친 뒤 문서만 갱신했습니다. 공개 Beta.6 commit `01b407ce81e9803878c003c181860e499718b0fa`와 구분합니다.

| 검사 | 최종 결과 | 확인한 범위 |
| --- | --- | --- |
| Codex live doctor | PASS | 실제 호출 1회, exit 0, structured answer 7, Skill 이름 8개, write probe 거부 |
| Claude live doctor | PASS | 최초 AUTH 실패 후 사용자 재인증과 추가 1회 승인. 재검증 exit 0, 같은 응답·read-only 조건 통과 |
| Homebrew 신규 설치 | PASS | 현재 QA source의 실제 source build, `brew test`, commit 및 darwin/arm64 metadata |
| Homebrew 업그레이드 | PASS | Beta.6 source → QA candidate revision 1, 실제 `brew upgrade`와 상태 보존 |
| `govulncheck` | PASS | Go 1.27.2, darwin/arm64 source symbol scan, exit 0, finding 0건 |

## 실제 provider 연결

사용자는 전체 refactoring cycle 대신 provider별 최소 live doctor를 선택했습니다. 요청 모델은 Codex `gpt-6.1-sol`과 Claude `claude-sonnet-5-5`, effort는 `low`입니다. 각 호출은 180초와 process 종료 grace로 제한했고 fallback은 사용하지 않았습니다. Claude의 호출당 USD 1 상한은 임시 프로젝트 설정에만 적용했습니다. Codex에는 금액 상한이 없어 사용량을 기록했습니다.

다음 명령을 현재 source에서 만든 binary와 임시 sparse clone에 실행했습니다. 새 commit 없이 기존 HEAD를 복사했고, `.agents`·`.claude` 제외가 doctor의 detached worktree에도 유지되는지 먼저 확인했습니다. 전역 Skill 검증은 그대로 실행했습니다.

```sh
QA_BIN=/private/tmp/refactor-me-live-qa-20261012/bin/refactor-me
QA_TARGET=/private/tmp/refactor-me-live-qa-20261012/provider/repo-copy
"$QA_BIN" doctor --repo "$QA_TARGET" --provider codex --fallback none \
  --model gpt-6.1-sol --effort low --json
"$QA_BIN" doctor --repo "$QA_TARGET" --provider claude --fallback none \
  --model claude-sonnet-5-5 --effort low --json
```

| 실제 호출 | Process exit | 측정 시간 | Input / output tokens | 비용 근거 |
| --- | ---: | ---: | --- | --- |
| Codex 1회 | 0 | 14.517초 | 100,984 / 270 | 보고 비용 없음. 기존 offline estimate USD 0.0816088 |
| Claude 최초 1회 | 1 | 1.241초 | 0 / 0 | provider 보고 USD 0; `AUTH`, OAuth 만료 |
| Claude 재인증 후 1회 | 0 | 5.266초 | 12,316 / 362 | provider 보고 USD 0.05288 |

총 실제 호출은 3회이며 자동 retry는 없었습니다. 최초 Claude 실패를 보존한 뒤 사용자가 `claude auth login`으로 인증을 갱신하고 같은 조건의 추가 1회를 승인했습니다. Agent는 인증 파일이나 계정 설정을 바꾸지 않았습니다. 보고 비용과 offline estimate는 청구 내역 확인을 뜻하지 않습니다.

두 provider의 최종 doctor는 각각 healthy 상태와 excluded 없음, answer `7`, Skill 이름 8개 및 `wrote_file=false`를 확인했습니다. 실제 probe 파일도 생성되지 않았습니다. Skill 가시성 응답은 provider 자기보고이며 모든 파일을 읽거나 Skill 절차를 수행했다는 독립 증명이 아닙니다. 모델 이름은 요청값입니다.

사후 검사에서 전역 Skill 8개의 canonical name/path/content hash, binary와 임시 config bytes, 원본·임시 HEAD와 commit count를 확인했습니다. 새 commit은 없고 checkout은 문서 편집 전까지 clean이었습니다. Detached probe worktree는 정상 제거했습니다. 최초 transcript 9개를 유지하고 재검증을 더한 총 13개의 크기·SHA-256·`0600` 권한을 확인했습니다.

## Homebrew 설치와 업그레이드

사용자는 기존 설치를 보존하는 격리 환경과 임시 formula의 `revision 1`을 선택했습니다. 현재 QA source와 공개 Beta.6은 같은 version 문자열을 쓰므로 revision으로 실제 upgrade를 유도했습니다. Repository version, formula template와 공개 tap은 변경하지 않았습니다.

Homebrew 7.0.9를 독립 clone하고 portable Ruby를 복사했습니다. Cellar, cache, logs, build temp와 XDG trust store를 분리했습니다. Go dependency는 정상 Homebrew bottle 처리로 설치했습니다. Temp tap은 `tap-new --no-git`으로 생성해 commit을 만들지 않았습니다.

초기 `/private/tmp` prefix에서는 Homebrew가 `bin/brew`에 쓰기가 가능한 sandbox를 탐지해 source build를 거부했습니다. Sandbox를 유지한 채 독립 prefix를 `/Users/Shared/refactor-me-qa-20261012-f0055f1-4e9b5a`로 옮겨 통과했습니다. 초기 tap 준비의 sandbox DNS 실패와 source build 실패도 로그에 남겼습니다.

격리된 `bin/brew`로 아래 순서를 실제 실행했습니다. 사이에 임시 formula를 해당 source로 교체했습니다. 다음은 실행 당시 경로의 명령 기록이며 기존 `/opt/homebrew/bin/brew`로 바꿔 실행하는 안내가 아닙니다.

```sh
QA_BREW=/Users/Shared/refactor-me-qa-20261012-f0055f1-4e9b5a/bin/brew
"$QA_BREW" install --build-from-source local/refactor-me-check/refactor-me
"$QA_BREW" test local/refactor-me-check/refactor-me
# 소유한 신규 candidate만 제거하고 Beta.6 baseline을 설치·검사
# 임시 formula에 QA source와 revision 1을 적용
"$QA_BREW" upgrade local/refactor-me-check/refactor-me
"$QA_BREW" test local/refactor-me-check/refactor-me
```

| 설치 단계 | CLI commit | Homebrew keg |
| --- | --- | --- |
| QA 신규 설치 | `f0055f1713544482f5d63bc3d30b0d79491d3bea` | `0.10.0-beta.6` |
| Beta.6 baseline | `01b407ce81e9803878c003c181860e499718b0fa` | `0.10.0-beta.6` |
| QA upgrade | `f0055f1713544482f5d63bc3d30b0d79491d3bea` | `0.10.0-beta.6_1` |

CLI version은 세 단계 모두 `0.10.0-beta.6`이며 `go1.27.2`, `darwin/arm64`를 보고했습니다. Upgrade log와 receipt에서 실제 upgrade 및 source build를 확인했습니다.

편집한 config schema 2, seeded report schema 3, last-run과 `.gitignore`의 snapshot은 **upgrade 직후 `init`/`report`까지 바이트 단위로 보존**됐습니다. 이후 별도 부정 입력으로 report를 schema 2로 바꿨고, nonzero exit·stdout 없음·입력 변경/삭제 없음으로 거부됐습니다. 최종 disposable report는 이 부정 입력이며 최초 schema 3과 같은 상태라는 뜻은 아닙니다. Seeded report는 실제 provider의 과거 실행 결과가 아닙니다.

검증 직후 snapshot에서는 기존 `/opt/homebrew`의 refactor-me binary·receipt·version·opt target, tap HEAD/status, trust hash 및 Homebrew checkout 상태가 전후 동일했습니다. 이후 소유한 `/Users/Shared` 검증 prefix를 정리한 시점에는 새 전역 tap `soom-kang/homebrew-sharpen-me`가 관찰됐습니다. 기존 tap 내용과 refactor-me·trust 상태는 그대로였고 새 tap은 보존했습니다. `[Unverified]` 추가 원인은 확인하지 못했으며 실행 기록에는 전역 tap 추가 명령이 없습니다.

이 검증은 [Tier 3 custom prefix](https://docs.brew.sh/Support-Tiers#custom-prefixes)의 로컬 candidate 결과입니다. 기본 prefix/Tier 1이나 공개 release 설치 인증으로 확대하지 않습니다. 검증 prefix는 정리했고 최종 formula, baseline/upgrade receipt, 명령 로그와 snapshot 근거는 임시 evidence 경로에 보존했습니다. 신규 설치 receipt 원본은 baseline 준비를 위해 candidate를 제거할 때 삭제됐으므로 측정 hash와 실행 기록만 남아 있습니다. 이를 retained receipt 원본으로 표시하지 않습니다.

## Vulnerability scan

별도 개발 검사 도구 `govulncheck v1.8.0`을 임시 경로에 설치했습니다. Product module에는 dependency를 추가하지 않았습니다. `tool/go`에서 공식 DB에 연결해 다음 검사를 실행했습니다.

```sh
/private/tmp/refactor-me-live-qa-20261012/bin/govulncheck -json ./...
```

Exit는 `0`, finding은 `0`건, stderr는 비어 있었습니다. Scanner config는 source/symbol, Go `go1.27.2`, DB `https://vuln.go.dev`, DB 수정 시각 `2026-10-08T22:31:09Z`입니다. JSON의 OSV record 186개는 DB 응답이며 finding 수가 아닙니다. Tests는 포함하지 않았습니다.

결과는 해당 시점의 알려진 취약점을 현재 platform과 source call graph에서 검사한 근거입니다. 다른 platform, provider CLI 자체, 전체 보안 감사 또는 알려지지 않은 취약점의 부재를 증명하지 않습니다. 검사 방식은 [Go 공식 설명](https://go.dev/doc/security/vuln/)을 따릅니다.

## 증거와 종료 범위

원본 로그는 저장소 밖에 보관하고 provider text와 인증 정보는 이 문서에 넣지 않았습니다. 구조화된 결과와 transcript/receipt/snapshot hash는 다음 경로에 있습니다.

```text
/private/tmp/refactor-me-live-qa-20261012/govulncheck.summary.json
/private/tmp/refactor-me-live-qa-20261012/provider/provider-verification-evidence.json
/private/tmp/refactor-me-live-qa-20261012/homebrew/evidence.json
```

| Artifact | SHA-256 |
| --- | --- |
| QA candidate source archive | `5263f3185a0ac07e5965cc0936c1b33a10d0d840858aeca7068f5e8ba75fc306` |
| Provider 검증 binary | `01dd5144193fcd036a68346e348f918aa0233bc9b4b643d63c309d3d65759f7b` |
| govulncheck 원본 JSON | `2c10bf6592288410af9bdb4c9d9f936758ae0c04fce2d76b71e67727ec1f00a0` |

Source runtime은 변경하지 않아 앞선 Go test/race/vet/lint 및 packaging 결과를 반복 실행하지 않았습니다. 이번 변경은 검증 기록과 문서 상태 갱신에 한정합니다. 문서에는 `i-have-adhd`, `humanizer`, `grammar-checker`, `style-guide`를 적용했으며 링크·공백·증거 범위를 검토했습니다.

합의한 검증은 완료했습니다. 전체 refactoring cycle, fallback·자동 retry·CLI SIGINT, 기본 prefix의 공개 신규 설치·upgrade, 새 standalone release artifact와 signing/notarization은 이번 범위 밖입니다. Commit·push·배포, workflow dispatch와 외부 JIRA 변경은 수행하지 않았습니다.
