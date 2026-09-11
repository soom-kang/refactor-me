![refactor-me](assets/refactor-me-title.png)

# refactor-me

[![Beta](https://img.shields.io/badge/release-v0.8.8--beta.1-orange)](https://github.com/soom-kang/refactor-me/releases/tag/v0.8.8-beta.1) [![Verify](https://github.com/soom-kang/refactor-me/actions/workflows/verify.yml/badge.svg)](https://github.com/soom-kang/refactor-me/actions/workflows/verify.yml) [![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](../LICENSE)

Codex와 Claude Code로 동작을 유지하는 리팩터링을 실행하는 CLI입니다. 별도 worktree에서 수정과 검증을 진행하고 결과를 `refactor/auto-*` 로컬 브랜치에 남깁니다. 병합 전에는 diff와 리포트를 확인하세요.

**공개 Beta `v0.8.8-beta.1`**: 안정판 전까지 인터페이스와 동작이 바뀔 수 있습니다.

[English](../README.md) · [실행 가이드](../tool/TUTORIAL.ko.md) · [상세 문서](../tool/README.ko.md) · [Workflow](../tool/WORKFLOW.ko.md) · [변경 이력](../tool/CHANGELOG.md)

## 설치

먼저 다음 조건을 준비하세요.

- macOS, Node.js 24 이상, Git
- 인증된 `codex` 또는 `claude` CLI와 네트워크 연결
- 별도로 릴리즈된 필수 의존성 [sharpen-me](https://github.com/soom-kang/sharpen-me)의 Skill 8개
- 의존성을 설치하고 빌드 캐시를 준비한 대상 프로젝트

1. Beta를 clone하고 도구를 설치하세요. 예시 경로는 대상 저장소의 경로로 바꾸세요.

```bash
git clone --branch v0.8.8-beta.1 --depth 1 https://github.com/soom-kang/refactor-me.git
cd refactor-me
node tool/install.mjs /path/to/target-repo
```

2. **대상 저장소**에서 Skill을 설치하고 Project 범위를 선택하세요.

```bash
cd /path/to/target-repo
npx skills add soom-kang/sharpen-me --skill '*' --agent codex claude-code
```

3. 설치한 Skill 파일, 에이전트 링크와 `skills-lock.json`을 검토하고 커밋하세요. 실행용 worktree는 기준 커밋을 읽으므로 미커밋 Skill을 사용할 수 없습니다. Claude 전역 설치만으로는 부족합니다.

설치기는 `.refactor/`에 도구를 복사하고 기존 설정과 실행 기록을 보존합니다. 도구 자체의 npm 패키지 설치나 빌드는 필요하지 않습니다.

## 실행

첫 실행 전에 [실행 가이드](../tool/TUTORIAL.ko.md#한도-설정과-실행)를 따라 한도를 정하세요. 대상 저장소에서 실행합니다.

```bash
./.refactor/bin/refactor-me version
./.refactor/bin/refactor-me doctor
./.refactor/bin/refactor-me --provider codex --fallback none
```

`doctor`는 진단 기록을 쓰고 모델을 호출해 계정 사용량을 소비합니다. `doctor --no-live-probe`는 모델 호출을 생략하며 세션의 Skill 로딩 여부는 확인하지 않습니다.

<details>
<summary>프로바이더 전환과 탐색 범위 옵션</summary>

```bash
./.refactor/bin/refactor-me --provider codex --fallback claude
./.refactor/bin/refactor-me --target app/web
./.refactor/bin/refactor-me --target app/web --target app/api
```

`--target`은 후보 탐색 범위입니다. 호출부 수정과 검증은 폴더 밖까지 이어질 수 있습니다. [범위와 검증 기준](../tool/README.ko.md#후보-선택)을 확인하세요.

</details>

실행 중에는 원본 checkout을 깨끗하게 유지하세요. CLI는 로컬 커밋과 결과 브랜치를 만들며 merge, push, 배포는 수행하지 않습니다.

## 리포트

실행 후 결과를 확인하세요. 기본 언어는 영어입니다.

```bash
./.refactor/bin/refactor-me report
./.refactor/bin/refactor-me report --lang ko
./.refactor/bin/refactor-me report --json
```

조회할 때 파일을 덮어쓰거나 모델을 호출하지 않습니다. **새 실행부터** 한국어를 사용하려면:

```bash
./.refactor/bin/refactor-me --lang ko
```

리포트에는 파일 통계, diff 미리보기와 전체 텍스트 patch인 `changes.patch`가 있습니다. 종료 상태와 변경을 검토한 뒤 병합하세요. [리포트 형식과 제한사항](../tool/README.ko.md#리포트와-언어), [단계별 Skill 역할](../tool/README.ko.md#skill-설치-상태)은 상세 문서에 있습니다.

## 검증

이 저장소 루트에서 실행하세요.

```bash
node --test tool/test/*.test.mjs
node tool/bin/refactor-me.mjs help
node tool/bin/refactor-me.mjs version --json
```

로컬 테스트는 설치와 CLI 동작을 확인합니다. 실제 모델의 판단 품질은 별도 실행으로 확인해야 합니다. [검증 범위](../tool/README.ko.md#로컬-개발-검사)를 참고하세요.

## 라이선스

CLI와 문서는 [MIT License](../LICENSE)를 적용하며 저작권자는 2026 soom-kang입니다. 설치된 CLI에도 `.refactor/lib/LICENSE`를 포함합니다. 필수 의존성인 sharpen-me Skill은 각자의 MIT 라이선스 파일을 유지합니다. Codex와 Claude Code는 외부 선행 도구로 각 공급자의 약관을 따릅니다.
