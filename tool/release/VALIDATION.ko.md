# 0.10.0-beta.1 로컬 검증 기록

검증일: 2026-09-29, macOS Apple Silicon, Go 1.27.1, Homebrew 7.0.6.

현재 상태는 **로컬 구현·패키지 검증 완료, 실제 provider 호출 승인 대기**다. 공개 배포 완료 기록이 아니다. 작업 시작 시 저장소는 깨끗했으며 기준 HEAD는 `c19b4e212563a774b532239af6ff774f6e1ae00c`였다. 아직 변경 사항을 commit하지 않았으므로 이 SHA를 새 릴리스 commit으로 사용해서는 안 된다.

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
| 실제 provider 세션 | NOT_RUN | 별도 호출 승인을 요청했으며 유료 모델 호출은 시작하지 않음 |
| 원격 CI | NOT_RUN | 변경을 push하지 않음 |
| 공개 설치 | NOT_RUN | prerelease와 전용 tap을 게시하지 않음 |

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

## 원격 확인과 남은 승인

읽기 전용 GitHub 확인에서 본 저장소의 기본 branch는 `main`, 현재 인증 주체의 관리 권한은 확인됐다. `soom-kang/homebrew-refactor-me`는 HTTP 404, `v0.10.0-beta.1` release는 미발견이다. 404만으로 저장소 생성 권한까지 확인했다고 판단하지 않는다.

1. **실제 호출 승인:** `/private/tmp/refactor-live-codex-010`, `/private/tmp/refactor-live-claude-010`에서 각각 doctor와 run. 최대 1 cycle·1 refactor commit, 호출당 300초·provider별 전체 20분. 전역 Skill은 읽기만 하고 fixture의 원본 HEAD·index·파일을 비교한다. 시간 제한은 금액 상한이 아니다.
2. **공개 변경 승인:** 실제 호출 결과가 통과한 뒤 변경 내용, 최종 commit, source hash, formula와 release notes를 검토한다. 승인 전에는 commit·push·tag·GitHub release·원격 tap 생성/수정을 진행하지 않는다.
3. **게시 후 확인:** 공개 소스로 Homebrew install/test를 실행하고 버전·commit·대상 상태 경로를 다시 확인한다.

Live probe의 Skill 목록은 provider의 세션 자기보고다. 측정한 파일/hash와 구분해서 평가하고 기록한다. 어느 provider든 필수 검증에 실패하면 공개를 보류한다.
