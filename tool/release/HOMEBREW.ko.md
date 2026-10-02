# Homebrew 릴리스 준비

[English](HOMEBREW.md) · [개발 검사](../DEVELOPMENT.md) · [사용자 설치](INSTALL.ko.md)

`soom-kang/homebrew-refactor-me`에서 macOS Apple Silicon용 소스 빌드 formula를 관리합니다. 생성기는 소스 URL, SHA-256, commit을 고정합니다. Homebrew가 빌드용 Go를 관리하며 Skills나 대상 프로젝트 설정은 설치하지 않습니다.

다음 후보는 `0.10.0-beta.2`이며 게시 전까지 공개 버전은 `0.10.0-beta.1`입니다. 작업 파일의 버전을 바꾸는 것은 릴리스 준비이며 게시 완료를 뜻하지 않습니다.

## 1. 로컬 후보 생성

macOS Apple Silicon, Git, Go 1.27+, Homebrew, Python 3가 필요합니다. 저장소 루트에서 새 출력 디렉터리를 지정합니다.

```sh
python3 -m unittest discover -s tool/release -p 'test_*.py'
python3 tool/release/prepare-homebrew.py \
  /tmp/refactor-homebrew-candidate --candidate
(cd /tmp/refactor-homebrew-candidate && shasum -a 256 -c SHA256SUMS)
```

후보 모드는 로컬 소스 변경을 포함하고 기준 commit과 미커밋 상태를 기록합니다. `file://` formula는 로컬 검증용입니다. 압축파일에는 소스, 내장 schema와 template, LICENSE, 빌드 정보를 넣고 `.git`과 사용자 Skills는 넣지 않습니다.

## 2. Formula 검사

refactor-me를 설치하지 않은 깨끗한 임시 macOS Apple Silicon runner를 사용합니다. 아래 명령은 후보를 설치하고 검증용 formula에만 trust를 설정합니다. 이미 refactor-me가 있다면 중단하고 다른 환경을 사용하세요. 사용자의 기존 설치와 trust 설정은 보존합니다.

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

[개발 검사](../DEVELOPMENT.md), lint, 취약점 검사를 실행합니다. 코드 변경은 독립적으로 검토하고 영·한 설치 안내는 별도 맥락에서 읽어 봅니다.

별도 승인한 제한 내에서 모델과 추론 수준을 명시한 Codex와 Claude fixture를 실행합니다. 모델은 fixture 설정에 지정할 수도 있습니다. 원본 checkout 불변성, Skill 경로·hash, 세션 로딩 결과, 제한, 결과, 사용량을 기록합니다. 예시 모델 ID는 사용자의 선택값이며 provider 지원을 검증한 값이 아닙니다. 파일 검사, 요청한 provider 설정, provider의 자기보고는 별개의 근거입니다. 필수 provider 검사가 실패하면 게시를 보류합니다.

후보 commit, 로컬 결과, CI 상태, 소스 hash, formula, 릴리스 노트를 승인 자료로 준비합니다. 원본 provider 로그에는 소스나 계정 정보가 포함될 수 있으므로 비공개로 보관합니다.

## 4. 승인한 버전 게시

원격 ref와 tap 접근 권한을 확인합니다. 승인할 commit을 만들기 전에 `tool/RELEASE_VERSION`에 새 버전을 쓰고 CHANGELOG에 해당 항목을 추가합니다. 기존 태그를 재사용하거나 이동하지 않습니다.

commit·push 승인을 받은 뒤 해당 commit에 `v<release-version>` 태그를 붙입니다. 그 commit의 깨끗한 checkout에서 실행합니다.

```sh
python3 tool/release/prepare-homebrew.py /tmp/refactor-homebrew-release
(cd /tmp/refactor-homebrew-release && shasum -a 256 -c SHA256SUMS)
bash tool/release/build-macos-arm64.sh /tmp/refactor-binary-release
```

릴리스 모드는 미커밋 변경이 있거나 태그가 HEAD를 가리키지 않으면 중단합니다. 두 출력 디렉터리의 소스 압축파일, 실행 파일 압축파일과 두 항목을 합친 `SHA256SUMS` 하나를 준비합니다. 두 hash를 함께 재검사한 뒤 draft prerelease에 올립니다. 확인한 변경과 제약으로 해당 버전의 릴리스 노트를 작성합니다. 태그 CI 통과 후 파일명과 hash를 확인하고 게시합니다.

소스 URL이 공개되면 생성한 `Formula/refactor-me.rb`를 tap에 반영합니다. 고정 hash와 주입한 commit을 유지합니다. bottle과 자동 tap 게시 기능은 없으며 Apple 서명·공증을 제공한다고 안내하지 않습니다. checksum은 파일 무결성을 확인하며 게시자를 인증하지 않습니다.

## 5. 공개 설치 확인

릴리스 자산을 공개하고 tap formula를 갱신한 뒤, 이 저장소에서 게시한 후보 태그를 지정해 workflow를 실행합니다.

```sh
gh workflow run verify.yml --ref v0.10.0-beta.2 -f public_install=true
```

해당 dispatch의 `go`, `public-install`, 집계 `verify`가 모두 성공했는지 확인하세요. 요청하지 않은 선택 항목 `js-fixture`는 생략할 수 있습니다. `public-install`에서 설치된 JSON 버전은 선택한 태그 checkout의 `tool/RELEASE_VERSION`, commit은 해당 checkout의 HEAD와 같아야 합니다. 플랫폼은 `darwin`, 아키텍처는 `arm64`여야 합니다. 다른 ref의 성공이나 `public_install=true`가 없는 실행은 이 릴리스 검사를 충족하지 않습니다. public-install 로그를 읽고 실행 URL을 근거로 남기세요.

refactor-me와 해당 tap이 없는 깨끗한 임시 runner를 사용합니다. tap을 등록하고 해당 formula만 신뢰한 뒤 짧은 설치 명령을 검사합니다.

```sh
brew tap soom-kang/refactor-me
brew trust --formula soom-kang/refactor-me/refactor-me
brew install refactor-me
brew test refactor-me
refactor-me version --json
```

공개 버전과 릴리스 commit을 확인한 뒤 임시 Git 저장소에서 대상 선택, 반복 `init`, 모델 호출 없는 doctor를 검사합니다. 보고서 조회는 저장된 fixture 보고서로 확인하며 설치 검사를 위해 provider 실행을 시작하지 않습니다. 원본 HEAD, index, 추적 파일은 유지되어야 합니다. 이 근거를 얻기 위해 사용자의 기존 설치를 교체하거나 관련 없는 Homebrew trust를 바꾸지 않습니다.

공개 설치를 통과한 뒤에만 영·한 설치 문서와 tap README 두 개의 공개 버전과 commit을 갱신하고 후보 안내를 제거합니다. 이는 저장소 문서를 갱신하며 이미 게시한 압축파일의 동봉 안내는 바꾸지 않습니다. 실제 main 저장소 workflow를 연결하는 `Verify` badge도 그때 추가합니다. tap README에는 `CLI Verify`로 표시하며 tap CI나 실제 provider 실행을 입증하는 배지로 안내하지 않습니다. 기존 타이틀 이미지를 유지합니다. 기존 사용자는 이후 `brew upgrade refactor-me`를 직접 실행할 수 있습니다. 문서만 갱신할 때는 태그, 자산, formula 버전을 새로 만들 필요가 없습니다.
