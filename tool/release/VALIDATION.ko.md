# 0.10.0-beta.1 로컬 검증 기록

검증일: 2026-09-29, macOS Apple Silicon, Go 1.27.1, Homebrew 7.0.6.

2026-09-29 추가 검증에서 **Codex·Claude 실제 실행이 모두 통과**했다. 사용자가 남은 검증과 최종 배포 진행을 승인했다. 검증한 구현 commit은 `f155cfee4bc298eaeaf519032ee73911e0f15987`이며, 이번 릴리스 준비 변경은 문서와 검증 결과 기록이다. 릴리스 commit은 `cf31fdf797a68d6bf5cab8a0f22eff9b55766071`이다. 아래에는 태그 생성 전 검증과 게시 후 공개 설치 결과를 구분해서 기록한다. 공개 asset의 commit·hash는 배포물의 BUILD-INFO와 SHA256SUMS로도 확인할 수 있다.

## 확인한 결과

| 항목 | 결과 | 근거·범위 |
| --- | --- | --- |
| 기본 Go 검증 | PASS | Node 차단 환경에서 `go test -race -count=1 ./...`, `go vet ./...`, `gofmt`, CLI build |
| Lint | PASS | golangci-lint 2.14.0, govet·ineffassign·unused 활성화, 0 issues |
| 취약점 검사 | PASS | govulncheck 1.8.0, 공식 데이터베이스 조회, `No vulnerabilities found` |
| CLI·저장소 선택 | PASS | CWD·`--repo`, 공백·symlink·target 경계, 반복 init, 상태 분리와 report 원문 출력 테스트 |
| 전역 Skills | PASS | 임시 HOME에서 누락·손상·동명 충돌·경로 탈출·hash 변경·Claude 복사본·provider 인자 테스트 |
| 실행 안전성 | PASS | 자식 프로세스 timeout, hook 변경, ref 충돌, 원본 checkout 보호, 지원하지 않는 상태로 clean 거부 |
| 소스 패키지 | PASS | Python unittest 3개. 결정적 압축, 기존 출력 거부, dirty 공개 거부, ignored 파일 제외 |
| Homebrew | PASS | 로컬 tap의 소스 빌드·재설치·`brew test`·`brew style`·`brew audit --strict` |
| 추가 ZIP 패키지 | PASS | 로컬 후보 ZIP 생성, arm64·버전·commit·checksum 확인 |
| 독립 구현 리뷰 | PASS | 발견한 3개 결함 수정 후 추가 finding 없음 |
| 문서 cold read | 보완 완료 | 별도 컨텍스트에서 EN/KO README·튜토리얼과 Homebrew 절차 검토. 개발 바이너리 경로 치환 안내 추가 |
| 실제 사용자 환경의 비호출 doctor | PASS | 서로 다른 임시 Go fixture에서 Codex·Claude 각각 `--no-live-probe --json`, `ok: true` |
| 실제 provider 세션 | PASS | Codex·Claude 각각 1 cycle·1 refactor commit, 원본 HEAD·index·추적 파일 불변, 결과 branch 확인 |
| 원격 CI | PASS | [태그 Verify](https://github.com/soom-kang/refactor-me/actions/runs/36502201222) 통과 후 게시 |
| 공개 설치 | PASS | 공개 URL 재다운로드·checksum 검증, 실제 brew install/test/audit, 두 저장소의 init/doctor/report 경로 분리 확인 |

기본 검증은 JavaScript 예제를 생성하고 명령 탐지를 확인하지만 선택적 Node 실행 검증은 포함하지 않는다. 별도 Mac 검증은 합의에 따라 제외했다.

## 검토에서 수정한 결함

- 공개 source archive의 파일 목록을 디스크 순회에서 Git의 확정 commit 추적 파일로 변경했다. `.gitignore`로 숨긴 로컬 파일이 배포물에 들어가지 않는다.
- `~/.agents/skills`에서 다른 디렉터리 이름으로 등록된 동일 Skill 이름도 충돌로 검사한다.
- 다른 Skill의 `SKILL.md`가 FIFO인 경우 파일 검사에서 무기한 대기하지 않는다.

관련 회귀 테스트를 저장소에 추가했다. 현재 동작을 검증하는 기존 migration 테스트는 contract 테스트로 이름을 정리해 유지했다.

## 로컬 후보

- 소스: `/private/tmp/refactor-homebrew-candidate-010-final/refactor-me_0.10.0-beta.1_source.tar.gz`
- SHA-256: `b04784217475af82bfd5565ccc69b0180accb52897e05d6f9f455475068516eb`
- Formula: 같은 디렉터리의 `refactor-me.rb`
- Build 정보: 같은 디렉터리의 `source.json`; `candidate: true`, `dirty: true`

검증용 Homebrew 설치와 `local/refactor-me-check` tap은 검사 후 제거했고, `tap-new`가 활성화한 Homebrew 개발 모드도 원복했다. 프로젝트 설정이나 전역 Skills는 삭제하지 않았다.

이 hash는 로컬 검사 대상의 식별자다. 공개 승인 후 clean commit에서 다시 생성한 source archive의 hash와 바꿔 쓸 수 없다. formula 템플릿과 생성 절차는 [HOMEBREW.md](HOMEBREW.md)에 있다.

## 실제 provider 검증

| Provider | 버전 | 전체 소요 시간 | 결과 | Input / output tokens | 보고된 비용 |
| --- | --- | ---: | --- | ---: | ---: |
| Codex | 0.157.1 | 393.2초 | 1 cycle·1 commit, DONE_PARTIAL | 878,653 / 9,946 | 미보고 |
| Claude | 2.1.283 | 239.3초 | 1 cycle·1 commit, DONE_PARTIAL | 456,140 / 18,657 | $1.5212012 |

소요 시간과 사용량은 별도 doctor 및 run을 합한 값이다. `DONE_PARTIAL`은 설정한 cycle 한도 1에 도달한 정상 중단이며 timeout이나 안전 규칙 위반이 아니다. 각 provider의 원본 checkout은 그대로이고 결과 branch의 OID가 보고서와 일치한다. 의도된 Go 테스트 실패 기준선은 성공으로 계산하지 않고 기존 실패가 유지되는지 비교했다.

Codex는 선택한 전역 Skill 절대 경로를 읽었고 Claude는 전용 복사본의 Skill을 호출했다. 파일 hash와 세션 자기보고는 별도 근거이며, 모든 참조 파일을 읽었다는 독립 증명으로 해석하지 않는다. 여덟 SKILL.md의 Git blob hash가 설치 안내의 sharpen-me `v0.9.0-beta.2`와 일치함을 확인했다. 자세한 식별자·hash·사용량은 [LIVE_RESULTS.json](LIVE_RESULTS.json)에 있다. 원시 모델 대화나 인증 정보는 공개 기록에 포함하지 않았다.

## 공개 절차

원격 `main`과 로컬 구현 commit이 일치하고 branch protection·ruleset은 없었다. 전용 tap은 404, 이번 버전 태그·릴리스는 미발견이었다. 이전 공개 태그와 자산은 수정하지 않는다.

사용자의 배포 승인에 따라 릴리스 commit과 annotated tag를 반영하고, source archive·ZIP·SHA256SUMS를 draft에 첨부했다. GitHub digest와 로컬 SHA-256 일치를 확인한 뒤 태그 CI 통과 후 prerelease를 공개했다.

## 게시 후 확인

- [v0.10.0-beta.1 prerelease](https://github.com/soom-kang/refactor-me/releases/tag/v0.10.0-beta.1): 2026-09-29 공개, draft=false, prerelease=true.
- [Homebrew tap](https://github.com/soom-kang/homebrew-refactor-me): 공개 저장소, formula commit `61500de`.
- Source SHA-256: `9a2867a0d4ae652df27e05c952964e4dc0db155df53a813f8a6c2000a0ca80a7`.
- ZIP SHA-256: `b556e0cef7966abcfcc86928070b79b84f1e3d84f4aa146efcbad82b6ab39990`.
- 인증 없는 공개 다운로드로 두 자산을 다시 받고 SHA256SUMS와 일치를 확인했다.
- `brew install soom-kang/refactor-me/refactor-me`, `brew test`, `brew audit --strict` 통과. 설치된 `/opt/homebrew/bin/refactor-me`가 버전 `0.10.0-beta.1`, commit `cf31fdf797a68d6bf5cab8a0f22eff9b55766071`, darwin/arm64를 보고했다.
- 공백이 있는 두 임시 Git 저장소에서 반복 init, `doctor --no-live-probe --target src`, 서로 다른 report JSON 원문 출력을 확인했다. 설정·기록은 각 `.refactor`에 분리되고 `.refactor/bin`은 생성되지 않았으며 원본 HEAD·index·추적 파일은 그대로였다.
- 공개 Homebrew 설치는 유지했다. 검증 도구가 활성화한 Homebrew 개발 모드는 원복했고, 사용자의 전역 Skill·설정 및 기존 공개 태그·자산은 변경하지 않았다.

게시 후 CLI 검증에서는 모델을 추가 호출하지 않았다. 별도 Mac 검증과 선택적 JavaScript 실행 검증은 이번 범위에서 제외했다.
