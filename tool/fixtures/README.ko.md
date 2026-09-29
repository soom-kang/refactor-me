# 로컬 리팩토링 fixture

[English](README.md) · [개발 검사](../DEVELOPMENT.md) · [Workflow](../WORKFLOW.ko.md)

후보 탐색, 검증 명령 발견과 기준선 처리를 확인할 임시 Git 저장소를 만듭니다. 생성기에는 Go 1.27과 Git이 필요합니다. 최초 커밋을 만든 뒤 저장소 경로를 출력하며 Codex나 Claude를 호출하지 않습니다.

## Go 예제

이 저장소 루트에서 실행하세요.

```bash
cd tool/go
FIXTURE="$(go run ./cmd/make-fixture)"
printf '%s\n' "$FIXTURE"
(
  cd "$FIXTURE"
  go vet ./...
  go build ./...
)
```

기본 fixture에는 미사용 코드, 문자열 레지스트리를 통해 호출되는 플러그인, 중복 로직, 큰 파일, 호환 코드와 의도된 실패가 있습니다. `go vet ./...`와 `go build ./...`는 통과(`GREEN`)해야 합니다. 다음 명령은 `TestKnownBaseline`에서 실패(`RED`)해야 합니다.

```bash
(cd "$FIXTURE" && go test -count=1 ./...)
```

이 실패는 생성기 오류가 아니라 검증에 사용할 입력입니다. 리팩토링 후보는 기존 통과를 유지하고 실패 signature를 추가하지 않아야 합니다. fixture를 통과시키려고 의도된 실패를 고치지 마세요. 통과한 명령이 하나도 없는 기준선은 audit으로 진행할 수 없습니다.

## 옵션과 목적지 보호

다음 예시는 `tool/go`에서 실행합니다. 목적지를 직접 지정하면 빈 디렉터리라도 이미 존재해서는 안 됩니다. 생성기는 기존 파일·디렉터리·심볼릭 링크를 삭제하지 않고 거부합니다. 목적지를 생략하면 새 임시 디렉터리를 만듭니다.

```bash
go run ./cmd/make-fixture /tmp/my-new-refactor-fixture --lang go --multi
```

`--multi`는 선택한 언어로 `app/api`와 `app/worker` 영역을 추가합니다. 두 영역은 각각 manifest를 가집니다. 생성기는 Skills를 설치하거나 복사하지 않습니다. 실제 provider 실행은 fixture의 기준 커밋과 독립적으로 `~/.agents/skills`에서 전역 Skills를 찾습니다.

잘못된 옵션이나 기존 목적지는 오류입니다. 진단을 확인하고 새 목적지를 선택하세요. 재시도하기 위해 기존 저장소를 지우지 마세요.

<a id="optional-javascript-example"></a>

## JavaScript 예제(선택)

파일 생성에는 Go를 사용하며, 생성한 검사를 실행할 때 Node.js 24+가 필요합니다. npm 의존성 설치는 필요하지 않습니다. 동작 방식 부록의 `.mjs` 경로는 이 예제를 기준으로 합니다.

```bash
# tool/go에서 실행
JS_FIXTURE="$(go run ./cmd/make-fixture --lang js)"
(
  cd "$JS_FIXTURE"
  node tools/lint.mjs
  node --test test/*.test.mjs
  node tools/build.mjs
)
(cd "$JS_FIXTURE" && node tools/typecheck.mjs)
```

Lint·test·build는 통과해야 합니다. Typecheck는 `src/broken.mjs`에서 의도적으로 실패하므로 해석 가능한 `RED` 기준선으로 취급하세요. `.mjs` 파일은 fixture 안에 생성되며 refactor-me CLI에 설치되지 않습니다. 원본 템플릿은 실행 파일이 아닌 `.txt`로 보관합니다.

생성기의 통합 테스트에서 JavaScript 실행까지 확인하려면 `tool/go`에서 실행하세요.

```bash
REFACTOR_TEST_JS=1 go test -count=1 ./internal/fixture
```

기본 테스트는 Node를 실행하지 않습니다. CI에서는 Verify를 수동 실행할 때 `js_fixture: true`를 선택해 JavaScript fixture 검사를 켭니다. 위 명령은 provider를 호출하거나 모델 판단 품질을 측정하지 않습니다. 실제 실행에는 [가이드](../TUTORIAL.ko.md)의 전역 Skill 설치, provider 인증과 실행 한도 설정이 별도로 필요합니다. 생성 후 `refactor-me init --repo "$FIXTURE"`와 `refactor-me doctor --repo "$FIXTURE" --no-live-probe`로 준비 상태를 확인하세요. `run` 시작은 사용량을 소비하는 별도 작업입니다.
