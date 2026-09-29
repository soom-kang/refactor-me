# Homebrew 릴리스 준비

[English](HOMEBREW.md) · [개발 검사](../DEVELOPMENT.md) · [사용자 설치](INSTALL.ko.md)

`soom-kang/homebrew-refactor-me`에서 macOS Apple Silicon용 소스 빌드 formula를 관리합니다. 생성기는 소스 URL, SHA-256, commit을 고정합니다. Homebrew가 빌드용 Go를 관리하며 Skills나 대상 프로젝트 설정은 설치하지 않습니다.

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

임시 로컬 tap을 사용합니다. 아래 명령은 로컬 Homebrew 설치 상태를 바꾸므로 기존 `refactor-me`가 있는지 먼저 확인하세요. 관련 없는 테스트를 위해 기존 설치를 교체하지 마세요.

```sh
brew tap-new local/refactor-me-check
cp /tmp/refactor-homebrew-candidate/refactor-me.rb \
  "$(brew --repository local/refactor-me-check)/Formula/refactor-me.rb"
brew style local/refactor-me-check/refactor-me
brew audit --strict local/refactor-me-check/refactor-me
brew install --build-from-source local/refactor-me-check/refactor-me
brew test local/refactor-me-check/refactor-me
refactor-me version --json
```

`brew test`는 provider 호출 없이 버전, commit, 초기화를 검사합니다. 게시 전에는 최종 HTTPS formula도 다시 확인합니다. 검증 후 후보 설치와 임시 tap만 제거하려면 다음 명령을 사용합니다.

```sh
brew uninstall local/refactor-me-check/refactor-me
brew untap local/refactor-me-check
```

## 3. 릴리스 검증

[개발 검사](../DEVELOPMENT.md), lint, 취약점 검사를 실행합니다. 코드 변경은 독립적으로 검토하고 영·한 설치 안내는 별도 맥락에서 읽어 봅니다.

별도 승인한 제한 내에서 Codex와 Claude fixture를 실행합니다. 원본 checkout 불변성, Skill 경로·hash, 세션 로딩 결과, 제한, 결과, 사용량을 기록합니다. 파일 검사와 provider의 자기보고는 별개의 근거입니다. 필수 provider 검사가 실패하면 게시를 보류합니다.

후보 commit, 로컬 결과, CI 상태, 소스 hash, formula, 릴리스 노트를 승인 자료로 준비합니다. 원본 provider 로그에는 소스나 계정 정보가 포함될 수 있으므로 비공개로 보관합니다.

## 4. 승인한 버전 게시

원격 ref와 tap 접근 권한을 확인합니다. 승인할 commit을 만들기 전에 `tool/RELEASE_VERSION`에 새 버전을 쓰고 CHANGELOG에 해당 항목을 추가합니다. 기존 태그를 재사용하거나 이동하지 않습니다.

commit·push 승인을 받은 뒤 해당 commit에 `v<release-version>` 태그를 붙입니다. 그 commit의 깨끗한 checkout에서 실행합니다.

```sh
python3 tool/release/prepare-homebrew.py /tmp/refactor-homebrew-release
(cd /tmp/refactor-homebrew-release && shasum -a 256 -c SHA256SUMS)
```

릴리스 모드는 미커밋 변경이 있거나 태그가 HEAD를 가리키지 않으면 중단합니다. 생성한 소스 압축파일과 checksum을 draft prerelease에 올립니다. 확인한 변경과 제약으로 해당 버전의 릴리스 노트를 작성합니다. 태그 CI 통과 후 파일명과 hash를 확인하고 게시합니다.

소스 URL이 공개되면 생성한 `Formula/refactor-me.rb`를 tap에 반영합니다. 고정 hash와 주입한 commit을 유지합니다. bottle과 자동 tap 게시 기능은 없으며 Apple 서명·공증을 제공한다고 안내하지 않습니다. checksum은 파일 무결성을 확인하며 게시자를 인증하지 않습니다.

## 5. 공개 설치 확인

refactor-me를 설치하지 않은 환경에서 실행합니다.

```sh
brew install soom-kang/refactor-me/refactor-me
brew test soom-kang/refactor-me/refactor-me
refactor-me version --json
```

기존 설치가 있다면 변경 내용을 검토한 뒤 `brew upgrade`를 사용합니다. 버전과 commit을 확인하고 임시 Git 저장소에서 대상 선택, 반복 `init`, 보고서 경로를 검사합니다. 원본 HEAD, index, 추적 파일은 유지되어야 합니다. 문서만 갱신할 때는 태그, 자산, formula 버전을 새로 만들 필요가 없습니다.
