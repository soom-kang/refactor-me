# Homebrew 릴리스 준비

[English](HOMEBREW.md) · [개발 검사](../DEVELOPMENT.md) · [사용자 설치](INSTALL.ko.md)

`soom-kang/homebrew-refactor-me`에서 macOS Apple Silicon용 소스 빌드 formula를 관리합니다. 생성기는 소스 URL, SHA-256, commit을 고정합니다. Homebrew가 빌드용 Go를 관리하며 Skills나 대상 프로젝트 설정은 설치하지 않습니다.

현재 공개 버전은 [`0.10.0-beta.4`](https://github.com/soom-kang/refactor-me/releases/tag/v0.10.0-beta.4)이며 commit은 `0032e594eaab3240d4dee1aa133be5b9d6eb3c42`입니다. 작업 파일의 버전을 바꾸는 것은 릴리스 준비이며 게시 완료를 뜻하지 않습니다. 아래 절차는 다음 릴리스에도 적용합니다.

## 1. 선택 검사: 로컬 후보 생성

macOS Apple Silicon, Git, Go 1.27+, Homebrew, Python 3가 필요합니다. 로컬 후보는 packaging이나 formula를 변경할 때 사용하며, 일반 릴리스는 3단계부터 진행할 수 있습니다. 저장소 루트에서 checkout 밖의 새 출력 디렉터리를 지정합니다.

```sh
python3 tool/release/prepare-homebrew.py \
  /tmp/refactor-homebrew-candidate --candidate
(cd /tmp/refactor-homebrew-candidate && shasum -a 256 -c SHA256SUMS)
```

후보 모드는 로컬 소스 변경을 포함하고 기준 commit과 미커밋 상태를 기록합니다. `file://` formula는 로컬 검증용입니다. 압축파일에는 소스, 내장 schema와 template, LICENSE, 빌드 정보를 넣고 `.git`과 사용자 Skills는 넣지 않습니다.

## 2. 선택 검사: Formula

후보 설치 확인이 필요한 formula나 packaging 변경에 사용합니다. refactor-me를 설치하지 않은 깨끗한 임시 macOS Apple Silicon runner를 사용합니다. 아래 명령은 후보를 설치하고 검증용 formula에만 trust를 설정합니다. 이미 refactor-me가 있다면 다른 환경을 사용하세요. 사용자의 기존 설치와 trust 설정은 보존합니다.

```sh
brew tap-new local/refactor-me-check
cp /tmp/refactor-homebrew-candidate/refactor-me.rb \
  "$(brew --repository local/refactor-me-check)/Formula/refactor-me.rb"
brew trust --formula local/refactor-me-check/refactor-me
brew style local/refactor-me-check/refactor-me
brew audit --strict local/refactor-me-check/refactor-me
brew install --build-from-source local/refactor-me-check/refactor-me
brew test local/refactor-me-check/refactor-me
refactor-me version --json
```

`brew test`는 provider 호출 없이 버전, commit, 초기화를 검사합니다. 게시 전에는 최종 HTTPS formula도 다시 확인합니다. 검증 후 후보 설치와 임시 tap만 제거하려면 다음 명령을 사용합니다.

```sh
brew uninstall local/refactor-me-check/refactor-me
brew untrust --formula local/refactor-me-check/refactor-me
brew untap local/refactor-me-check
```

## 3. 릴리스 검증

[개발 검사](../DEVELOPMENT.md)의 `gofmt`, `go test -count=1 ./...`, `go vet ./...`, CLI build와 Python release test 4개가 필수 코드 검사입니다. PR과 main CI에서 이 검사를 실행합니다. 태그를 만들기 전에 `main` push로 시작한 `Verify` 실행이 성공했고, 그 `headSha`가 릴리스 commit과 같은지 확인한 뒤 실행 URL을 기록합니다. 다른 commit의 성공 결과로 대체할 수 없으며 별도 태그 CI는 필요하지 않습니다.

동일한 소스에는 이 결과를 재사용하고 검사를 반복하지 않습니다. race test, lint, 취약점 검사, formula audit, 독립 검토, 설치 안내 cold read는 변경한 동작이나 해결되지 않은 실패에 필요할 때만 추가합니다. 실제 Codex/Claude fixture는 provider 통합 변경을 확인하는 선택 검사이며, 승인된 범위와 실행 제한·사용량을 기록해야 합니다. 원본 provider 로그에는 소스나 계정 정보가 포함될 수 있으므로 비공개로 보관합니다.

릴리스 commit, 필수 CI 결과, 압축파일 hash, formula, 릴리스 노트를 기록합니다. commit, push, 릴리스 게시와 tap 갱신에는 기존 승인을 적용하고, 그 범위를 벗어나는 작업만 추가 승인을 받습니다.

## 4. 승인한 버전 게시

원격 ref와 tap 접근 권한을 확인합니다. 릴리스 commit을 만들기 전에 `tool/RELEASE_VERSION`에 새 버전을 쓰고 CHANGELOG에 해당 항목을 추가합니다. 기존 태그를 재사용하거나 이동하지 않습니다.

해당 commit의 main CI가 통과하면 `v<release-version>` 태그를 붙입니다. 그 commit의 깨끗한 checkout에서 소스를 먼저, 실행 파일을 다음으로 각각 한 번 생성합니다. checkout 밖의 새 출력 디렉터리를 사용합니다.

```sh
python3 tool/release/prepare-homebrew.py /tmp/refactor-homebrew-release
bash tool/release/build-macos-arm64.sh /tmp/refactor-binary-release
```

소스 릴리스 생성에는 깨끗한 checkout과 HEAD를 가리키는 로컬 버전 태그가 필요합니다. 실행 파일 빌드가 끝날 때까지 HEAD와 소스를 유지합니다. 실행 파일 생성기는 내장된 버전, commit, 아키텍처를 검사합니다. 두 출력 디렉터리의 소스 압축파일, 실행 파일 압축파일과 두 항목을 합친 `SHA256SUMS` 하나를 준비합니다. `shasum -a 256 -c SHA256SUMS`로 두 hash를 함께 검사한 뒤 draft prerelease에 올립니다. 확인한 변경과 제약으로 해당 버전의 릴리스 노트를 작성합니다. 업로드한 파일명과 hash를 확인하고 기록한 main CI 결과를 근거로 게시합니다. 태그 push는 별도 테스트를 시작하지 않습니다.

소스 URL이 공개되면 생성한 `Formula/refactor-me.rb`를 tap에 반영합니다. 고정 hash와 주입한 commit을 유지합니다. bottle과 자동 tap 게시 기능은 없으며 Apple 서명·공증을 제공한다고 안내하지 않습니다. checksum은 파일 무결성을 확인하며 게시자를 인증하지 않습니다.

## 5. 공개 설치 확인

릴리스 자산을 공개하고 tap formula를 갱신한 뒤, 이 저장소에서 게시한 릴리스 태그를 지정해 workflow를 실행합니다.

```sh
release_tag="v$(cat tool/RELEASE_VERSION)"
gh workflow run verify.yml --ref "$release_tag" -f public_install=true
```

해당 dispatch의 `public-install`과 집계 `verify`가 성공하고 `go`는 생략됐는지 확인하세요. 선택 항목 `js-fixture`도 요청하지 않았다면 생략됩니다. `public-install`에서 설치된 JSON 버전은 선택한 태그 checkout의 `tool/RELEASE_VERSION`, commit은 해당 checkout의 HEAD와 같아야 합니다. 플랫폼은 `darwin`, 아키텍처는 `arm64`여야 합니다. 다른 ref의 성공이나 `public_install=true`가 없는 실행은 이 릴리스 검사를 충족하지 않습니다. public-install 로그를 읽고 실행 URL을 근거로 남기세요.

workflow는 refactor-me가 없는 깨끗한 임시 runner에서 아래 설치를 한 번 실행합니다. tap을 등록하고 해당 formula만 신뢰한 뒤 짧은 설치 명령을 검사합니다.

```sh
brew tap soom-kang/refactor-me
brew trust --formula soom-kang/refactor-me/refactor-me
brew install refactor-me
brew test refactor-me
refactor-me version --json
```

`brew test`가 버전, commit, 도움말, 대상 저장소 선택, 기본 설정과 반복 `init`의 설정 보존을 이미 검사합니다. workflow는 설치된 버전과 commit을 선택한 릴리스 태그와도 비교합니다. 로컬에서 같은 검사를 반복하거나 설치 확인을 위해 실제 provider를 호출하지 않습니다. 사용자의 기존 설치와 관련 없는 Homebrew trust는 보존합니다.

공개 설치를 통과한 뒤에만 영·한 설치 문서와 tap README 두 개의 공개 버전과 commit을 갱신하고 후보 안내를 제거합니다. 이는 저장소 문서를 갱신하며 이미 게시한 압축파일의 동봉 안내는 바꾸지 않습니다. 실제 main 저장소 workflow를 연결하는 `Verify` badge도 그때 추가합니다. tap README에는 `CLI Verify`로 표시하며 tap CI나 실제 provider 실행을 입증하는 배지로 안내하지 않습니다. 기존 타이틀 이미지를 유지합니다. 기존 사용자는 이후 `brew upgrade refactor-me`를 직접 실행할 수 있습니다. 문서만 갱신할 때는 태그, 자산, formula 버전을 새로 만들 필요가 없습니다.

## Beta.4 검증 기록

릴리스 commit은 `0032e594eaab3240d4dee1aa133be5b9d6eb3c42`입니다. [Main CI](https://github.com/soom-kang/refactor-me/actions/runs/37207945672)와 [선택한 태그의 공개 설치 검증](https://github.com/soom-kang/refactor-me/actions/runs/37208178549)을 확인하세요.

- Main CI: 릴리스 commit과 같은 SHA에서 `PASS`. formatting, Go tests, vet, CLI build와 Python release test 4개를 검사했습니다. 태그 CI는 중복 실행하지 않았습니다.
- 공개 설치: `PASS`. `brew install`과 `brew test`를 통과했고 설치된 버전과 commit이 선택한 릴리스 태그와 일치했습니다. 플랫폼은 `darwin`, 아키텍처는 `arm64`입니다. `public-install`과 집계 `verify`가 성공했고 `go`와 `js-fixture`는 생략됐습니다.
- 소스 압축파일, 실행 파일 압축파일과 두 항목을 합친 `SHA256SUMS`를 게시 전에 검증했습니다.
- 이번 릴리스의 실제 Codex/Claude fixture와 HAPJOO 전체 재실행은 `NOT_RUN`입니다. 위 검사가 해당 프로젝트의 리팩토링 완주를 입증하지는 않습니다.

## Beta.3 검증 기록

릴리스 commit은 `01a3fad54149cb127e8bd6c5d638eda0562a5897`입니다. [태그 CI](https://github.com/soom-kang/refactor-me/actions/runs/36970731411)와 [선택한 태그의 공개 설치 검증](https://github.com/soom-kang/refactor-me/actions/runs/36971167653)을 확인하세요.

- 선택한 릴리스 태그의 공개 설치 결과: `PASS`. 필수 성공 job은 `go`, `public-install`, 집계 `verify`입니다. 이 실행은 공개 formula의 style, audit, 설치, 테스트와 버전·commit 일치를 검사합니다. 설치 검사는 provider를 호출하지 않습니다.
- Codex: 수락 기준 12개 모두 `PASS`. 검토를 통과한 `DEAD_CODE` 커밋 1개, 단계마다 1회씩 총 6회 CLI 호출, 재시도와 fallback 없음; 1,108.8초; 1 cycle 제한에 따른 `DONE_PARTIAL`. 요청 설정은 `gpt-6.1-sol`/`xhigh`, 리팩토링 단계의 provider 호출 제한 900초, 최대 시도 3회, 리팩토링 커밋 최대 1개와 작업 단위 사이에서 확인하는 20분 경과 시간 제한입니다. `run`에 포함된 doctor만 실행했으며 live probe 제한은 180초입니다. 집계한 보고 사용량은 input 987,494 tokens, output 27,695 tokens이며 `costUsd=null`로 비용은 미확인입니다.
- 불변성: clean 릴리스 소스 `01a3fad54149cb127e8bd6c5d638eda0562a5897`. 최종 실행 파일 SHA-256 `b55f0af5e389dd611772ac5592295aa0d6619c5910f6703944570737c303d911`는 실제 검증한 실행 파일과 같습니다. 측정한 runtime 파일 94개와 전역 Skills 8개가 모두 유지됐고 원본 fixture의 HEAD, index, 추적 파일과 설정도 보존됐습니다. `go vet`와 `go build`는 GREEN을 유지하고 `go test`에는 의도된 RED인 `GOFAIL:TestKnownBaseline`, `contract_test.go:7`만 남았습니다. Skill 로딩 응답은 provider의 자기보고이며 파일 hash는 별도로 측정했습니다.
- Claude: 사용자 수동 검증 `PASS`. 토큰 소진으로 자동 검증은 `NOT_RUN`이며 Beta.3 준비 중 Claude를 자동 호출하지 않았습니다.
- 원본 로그는 비공개로 보관합니다. Codex 결과는 위에서 설명한 제한된 fixture에 해당합니다.

## Beta.2 검증 기록

릴리스 commit은 `c30dbdadc4229958e29366fdc14d79df3da8cb02`입니다. [선택한 태그의 공개 설치 검증](https://github.com/soom-kang/refactor-me/actions/runs/36956289983)을 확인하세요. 아래 근거는 이번 릴리스와 제한된 fixture에 해당하며 모든 대상 저장소나 provider 계정을 보증하지 않습니다.

- main과 태그 CI는 Go 구현과 집계 `verify`를 검사합니다. 선택한 태그의 공개 설치 실행은 `public-install`도 성공해야 합니다. 공개 formula의 `brew style`, `brew audit --strict`, 설치, 테스트를 실행하고 버전과 commit을 선택한 checkout과 비교합니다. 선택 항목 `js-fixture`는 생략할 수 있으며 설치 검사는 provider를 호출하지 않습니다.
- Codex: `PASS; 검토된 커밋 1개, 호출 6회, 재시도 없음; 1,049.1초; cycle 제한에 따른 DONE_PARTIAL`. 새 Go fixture에서 추가 run 1회를 실행했습니다. 요청 설정은 `gpt-6.1-sol`/`xhigh`, fallback 없음이며 호출 제한은 900초, 기존 최대 시도 횟수는 3회입니다. 이전 300초 audit 시도 3회는 사용할 수 있는 응답 없이 시간 초과했으며 그 실패 기록을 유지합니다. 별도 standalone doctor는 추가하지 않았습니다. 검증 실행 파일은 clean commit `265d3547b2541ce695f2ee8cedfab7225916cc18`에서 빌드했으며 `tool/go`의 모든 runtime 파일이 릴리스 commit과 같습니다.
- Claude: `tool/go`의 파일 85개와 Skill 8개의 `name/path/sha256` 기록이 릴리스 소스 및 catalog와 모두 같은지 확인한 뒤 기존 통과 fixture를 재사용했습니다. 요청 설정은 `claude-sonnet-5-5`/`xhigh`, fallback 없음이었으며 검토를 통과한 리팩토링 커밋 1개를 만들었습니다.
- 각 fixture는 1 cycle, 리팩토링 커밋 최대 1개로 제한합니다. 20분 제한은 작업 단위 사이에서 확인하며 전체 프로세스의 강제 마감 시간이나 금액 상한이 아닙니다. 통과하려면 검토된 리팩토링 커밋 1개가 있어야 합니다. `go vet ./...`와 `go build ./...`는 GREEN을 유지하고, `go test -count=1 ./...`에는 의도된 RED인 `GOFAIL:TestKnownBaseline`, `contract_test.go:7`만 허용합니다. 원본 HEAD, index, 추적 파일과 설정은 보존해야 합니다.
- `providerSettings`는 요청한 모델과 추론 수준이며 실제 실행을 관측한 값이 아닙니다. live Skill 로딩 응답은 provider의 자기보고이며 파일 hash 측정은 별도 근거입니다. 원본 provider 로그는 비공개로 보관합니다. 제한된 fixture의 성공이 모델 품질이나 모든 환경의 호환성을 입증하지는 않습니다.
