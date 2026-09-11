# 리팩터링 Workflow

[프로젝트](../docs/README.ko.md) · [English](WORKFLOW.md) · [상세 문서](README.ko.md) · [실행 가이드](TUTORIAL.ko.md)

먼저 전체 흐름을 확인하고 필요한 단계의 JSON 예시를 펼쳐 보세요. 사용자가 설정과 실행을 시작하면 컨트롤러가 프로바이더의 JSON 응답을 검사해 진행 여부를 결정합니다. 단계마다 사용자 승인을 받지는 않습니다.

예시는 [로컬 fixture](fixtures/make-fixture.mjs)의 `src/legacy-parser.mjs` 삭제입니다. `parseLegacy`와 `legacyVersion`을 내보내지만 기존 진입점의 호출은 없습니다. 같은 fixture의 `plugin-x`는 레지스트리가 이름으로 불러오므로 정적 import 검색만으로는 참조를 찾을 수 없습니다.

**JSON은 설명용 예시이며 실제 응답이나 실행 성공 기록이 아닙니다.** 한영판의 예시는 같고 현재 [응답 스키마](src/schemas.mjs)를 따릅니다. 질문은 [단계별 프롬프트](src/prompts.mjs)를 요약했습니다.

## 전체 흐름

![전체 흐름: 초기화, 기준선, 후보 조사, 실행 준비, 구현, 검증, 리뷰, 반영과 최종 리포트.](../docs/assets/workflow/overview.ko.png)

[HTML 원본](../docs/assets/workflow/overview.ko.html) · [원본 흐름과 대응 기록](../docs/assets/workflow/README.md)

컨트롤러는 작업 사이에 실행 한도와 프로바이더 상태도 확인합니다. 일반적인 후보 거절은 수정 직전 상태로 복원한 뒤 재조사하거나 종료합니다. 안전 정지는 worktree를 증거로 보존합니다.

## 1. INIT, doctor와 기준선

**입력:** 저장소, 설정, 프로바이더와 필수 Skill 8개를 확인합니다. Doctor의 모델 호출은 세션에서 Skill을 읽는지 검사하며 audit과 다른 진단 응답을 받습니다. `doctor --no-live-probe`는 세션 확인을 생략합니다.

**검사와 다음 단계:** 필수 조건을 충족하지 못하면 중단합니다. 초기화 후 별도 worktree에서 기준선 명령을 실행합니다. `.refactor/commands.json`에 고정한 목록이 없으면 명령을 자동 탐색합니다. 발견한 영역의 선택된 명령 중 하나 이상이 `GREEN`이어야 audit으로 진행합니다.

`src/broken.mjs`는 해석 가능한 typecheck 실패(`RED`)를 보여주는 fixture입니다. 다른 명령은 통과할 수 있습니다. 이는 설계 설명이며 이 문서에서 측정한 결과가 아닙니다.

이후 검증은 기존 통과를 유지하고 해석 가능한 실패에 새 오류 signature를 추가하지 않아야 합니다. 기준선의 `TIMEOUT`, `UNRUNNABLE`, `OPAQUE` 명령은 비교 근거가 없어 제외합니다.

## 2. Audit과 후보 정렬

**입력:** “범위가 제한된 동작 보존 리팩터링을 찾아라. 정적·동적 도달성, 외부 소비자, 관찰 가능한 동작과 위험도를 확인하고 추측에 의존한 후보는 제외하라.” 세션은 `sharpen-clarify`, `sharpen-review`, `sharpen-challenge`, `sharpen-assess`를 사용합니다.

**응답:** Legacy parser를 `DEAD_CODE`, `L0_LOW`, `READY`로 제안하고 참조 부재 근거를 반환합니다. `EXPORTED_UNUSED`만으로 외부 소비자가 없다고 판단할 수는 없습니다.

**검사와 다음 단계:** `rank`는 대상 범위, `UNKNOWN`과 허용하지 않은 위험도, `REJECT`, 처리 이력, 시도 횟수, 파일 수 한도를 검사합니다. 남은 후보는 위험도 → 준비 상태 → 파일 수 → 후보 ID 순서로 정렬합니다.

Audit의 `NEEDS_EVIDENCE`도 deep check로 진행할 수 있습니다. 선택한 후보의 시도 횟수를 기록하고 빈 audit이 설정 횟수만큼 반복되면 종료합니다. 기본값은 두 번입니다.

<details>
<summary>전체 JSON 응답</summary>

<!-- example: audit-success schema: audit -->
```json
{
  "schema_version": "1",
  "scan_scope": "Tracked source, tests, configuration and package surface in the fixture.",
  "rejected_count": 1,
  "notes": "Illustrative response, not a captured run. The plugin-x registry entry rules out deleting that plugin.",
  "candidates": [
    {
      "candidate_id": "dead-legacy-parser",
      "category": "DEAD_CODE",
      "title": "Remove the unreferenced legacy parser",
      "related_files": [
        "src/legacy-parser.mjs"
      ],
      "problem": "No existing entrypoint reaches this module.",
      "minimal_change": "Delete src/legacy-parser.mjs.",
      "observable_contracts": [
        "Keep the exports and behavior of src/index.mjs unchanged."
      ],
      "static_reachability": "EXPORTED_UNUSED",
      "dynamic_reachability": "NONE",
      "external_consumer_risk": "NONE",
      "ui_impact": "NONE",
      "persistence_impact": "NONE",
      "async_side_effect_impact": "NONE",
      "test_protection": "PARTIAL",
      "characterization_needed": false,
      "estimated_file_count": 1,
      "primary_symbol": "parseLegacy",
      "risk_level": "L0_LOW",
      "readiness": "READY",
      "evidence": [
        {
          "file": "src/legacy-parser.mjs",
          "locator": "parseLegacy and legacyVersion exports",
          "kind": "DEFINITION",
          "supports": "The module contains only the legacy parser and version helper."
        },
        {
          "file": "src/index.mjs",
          "locator": "Repository-wide search of imports, strings, registry entries, tests and configuration",
          "kind": "GREP_ABSENCE",
          "supports": "No reference reaches legacy-parser, parseLegacy or legacyVersion from an existing entrypoint."
        },
        {
          "file": "package.json",
          "locator": "private and entrypoint fields",
          "kind": "CONFIG",
          "supports": "The fixture has no declared public package surface."
        }
      ]
    }
  ]
}
```

</details>

## 3. Deep check와 작업 명세

**입력:** “현재 저장소의 근거로 후보를 검증하라. 최소 allowlist, 보존할 동작, 중단 조건과 필요한 characterization 테스트를 명시하라.” 세션은 `sharpen-review`와 `sharpen-challenge`를 사용합니다. 중복 제거 후보에는 `sharpen-dedupe`도 추가합니다.

**응답:** `READY`, 삭제할 파일 하나, 계약 `C1`, `characterization_needed: false`를 반환합니다.

**검사와 다음 단계:** `fp`와 `original_lines`를 추가해 `packet.json`을 저장합니다. `READY`이고 allowlist가 비어 있지 않아야 진행합니다. 예시는 preflight로 넘어갑니다. 테스트가 필요한 경우는 [characterization 분기](#characterization만-반영되는-경우)를 확인하세요.

<details>
<summary>전체 JSON 응답</summary>

<!-- example: deepcheck-success schema: deepcheck -->
```json
{
  "schema_version": "1",
  "candidate_id": "dead-legacy-parser",
  "category": "DEAD_CODE",
  "title": "Remove the unreferenced legacy parser",
  "minimal_change": "Delete src/legacy-parser.mjs.",
  "allowlist": [
    "src/legacy-parser.mjs"
  ],
  "forbidden_files": [
    "src/registry.mjs",
    "src/plugin-x.mjs"
  ],
  "contracts": [
    {
      "contract_id": "C1",
      "statement": "Existing src/index.mjs exports, return values and side effects stay unchanged.",
      "kind": "RETURN_SHAPE",
      "verified_by": "Inspect reachable imports and run the existing entrypoint tests and build."
    }
  ],
  "stop_conditions": [
    "Stop if an existing entrypoint or package consumer reaches the module.",
    "Stop if removal requires edits outside the allowlist."
  ],
  "rollback": {
    "strategy": "GIT_RESET_HARD",
    "detail": "The controller restores only the detached run worktree to its recorded pre-write commit."
  },
  "risk_level": "L0_LOW",
  "characterization_needed": false,
  "characterization_files": [],
  "readiness": "READY",
  "readiness_reason": "The fixture has no reachable consumer of this module; the existing entrypoint checks cover the retained behavior.",
  "notes": "Illustrative task packet. The controller adds fp and original_lines when saving packet.json."
}
```

</details>

## 4. Preflight

![실행 전 조건: deep check, 필요한 characterization 테스트, 별도 테스트 커밋과 preflight 판정.](../docs/assets/workflow/preparation.ko.png)

[HTML 원본](../docs/assets/workflow/preparation.ko.html) · [원본 흐름과 대응 기록](../docs/assets/workflow/README.md)

**입력:** “이 작업 명세가 실패할 수 있는 구체적인 가설을 하나 세우고, 그 가설을 반증해 보라.” 세션은 `sharpen-challenge`를 사용하며 코드 수정 없이 작업 명세, 저장소 정보와 기준선 요약을 읽습니다.

**응답:** 문자열 로더 가설은 `FALSIFIED`, 판정은 `READY_TO_EXECUTE`이며 차단 사유는 없습니다.

**검사와 다음 단계:** `preflight.json`을 저장하고 세 조건을 확인한 뒤 구현으로 진행합니다. 판정이 `READY_TO_EXECUTE`여도 가설이 `SURVIVED`나 `INCONCLUSIVE`이면 차단합니다.

<details>
<summary>전체 JSON 응답</summary>

<!-- example: preflight-success schema: preflight -->
```json
{
  "schema_version": "1",
  "candidate_id": "dead-legacy-parser",
  "verdict": "READY_TO_EXECUTE",
  "failure_hypothesis": "A string-based loader still resolves the legacy parser.",
  "falsification_method": "Inspect the registry, imports, scripts and configuration for the module path and exported symbols.",
  "falsification_result": "FALSIFIED",
  "blocking_reasons": [],
  "notes": "Illustrative evidence: registry references plugin-x, not the legacy parser."
}
```

</details>

## 5. 구현

**입력:** “확정한 작업 명세만 적용하라. Hunk별 성격을 분류하고 범위 확대가 필요하면 명시하라.” 세션은 `sharpen-refine`을 사용합니다. 중복 제거에서는 `sharpen-dedupe`도 허용합니다.

**응답:** `deleted_files`와 계약 `C1`을 연결하고 범위 확대 없이 `PASS`를 반환합니다. 파일 삭제, Git 작업과 검증 명령은 컨트롤러가 담당합니다.

**검사와 다음 단계:** `execution.json`을 저장합니다. `UNEXPLAINED`나 `CONTRACT_CHANGING` hunk, 범위 확대, 범위 밖 경로 선언은 `PASS`여도 거절 사유가 됩니다. 컨트롤러는 allowlist를 확인해 파일을 삭제한 뒤 gate에서 실제 diff를 검사합니다.

<details>
<summary>전체 JSON 응답</summary>

<!-- example: execute-success schema: execute -->
```json
{
  "schema_version": "1",
  "candidate_id": "dead-legacy-parser",
  "changed_files": [],
  "deleted_files": [
    "src/legacy-parser.mjs"
  ],
  "rationale": "Remove the module that no existing entrypoint reaches.",
  "hunks": [
    {
      "hunk_id": "H1",
      "file": "src/legacy-parser.mjs",
      "classification": "INTERNAL_ONLY",
      "justification": "The removed exports have no reachable consumer in the fixture.",
      "contract_ids": [
        "C1"
      ]
    }
  ],
  "verdict": "PASS",
  "scope_expansion_required": false,
  "scope_expansion_reason": null,
  "notes": "The controller applies the declared deletion and checks the resulting Git diff."
}
```

</details>

## 6. Diff gate와 검증

**검사:** 모델 호출 없이 실제 diff를 작업 명세와 비교합니다. 허용·금지 경로, 바이너리 변경, 변경량, 유형별 규칙, 테스트 약화, 이전 tree와 원본 checkout 상태를 검사해 `gate.json`을 저장합니다.

**다음 단계:** `NO_OP`이면 건너뛰고 gate 위반이면 수정을 되돌립니다. 안전 불변식 위반 시에는 실행을 멈추고 증거를 보존합니다. 통과하면 검증 명령을 실행해 `validation.json`을 저장합니다. 회귀나 판단 불가 결과가 나오면 리뷰 전에 거절합니다.

후보 검증은 변경 경로별로 가장 깊은 프로젝트 영역을 선택하고 루트 영역이 있으면 함께 검사합니다. 모든 영역을 매번 검증하지는 않습니다. 기준선은 탐색한 명령을 실행하고 도달성은 저장소 전체에서 확인합니다. `--target` 밖의 호출부 수정이나 선택한 폴더보다 넓은 루트 검증이 필요할 수 있습니다.

`accepted.patch`는 검증 후 **리뷰 전에** 저장합니다. 거절한 후보에도 남는 리뷰 입력입니다. 최종 반영 여부는 `review.json`, 반영한 커밋과 `changes.patch`로 확인하세요.


## 7. 독립 리뷰

**입력:** “작업 명세와 diff를 보고 동작을 보존했는지 판단하라.” 세션은 `sharpen-cold-review`를 사용합니다. 구현 이유는 받지만 구현 모델의 판정과 hunk 분류는 받지 않습니다. 별도 세션에서 리뷰하며, cross-provider review를 켰고 다른 프로바이더를 사용할 수 있으면 그 프로바이더를 우선합니다.

**응답:** `PASS`, `PRESERVED`, 빈 findings와 unreviewable_hunks를 반환합니다.

**검사와 다음 단계:** `review.json`을 저장합니다. `BLOCKER`, `PRESERVED`가 아닌 평가, 모델의 `FAIL`은 거절 사유입니다. 거절하면 구현 직전 커밋으로 복원합니다.

**`HIGH`만으로 `PASS`가 바뀌지는 않습니다.** 판정과 함께 findings를 확인하세요.

<details>
<summary>전체 JSON 응답</summary>

<!-- example: review-success schema: review -->
```json
{
  "schema_version": "1",
  "candidate_id": "dead-legacy-parser",
  "verdict": "PASS",
  "behavior_preservation_assessment": "PRESERVED",
  "unreviewable_hunks": [],
  "findings": [],
  "notes": "Illustrative review: deletion is confined to the unreferenced module; existing entrypoints remain unchanged."
}
```

</details>

## 8. 커밋, 재조사와 리포트

**반영:** 리뷰한 tree와 커밋할 tree가 같아야 합니다. 로컬 커밋을 만들고 결과 브랜치의 이전 OID가 예상값과 같을 때 갱신합니다. `state.publishedOid`와 커밋을 기록한 뒤 한도 안에서 재조사합니다. 원본 checkout이나 승인한 tree가 바뀌면 반영을 중단할 수 있습니다.

**결과:** 종료 시 `report.json`과 선택한 언어의 `report.md`를 저장합니다. 원본 저장소의 `state.baseOid`와 `state.publishedOid`를 비교하며 characterization 커밋도 포함합니다. Worktree 보존 여부나 현재 브랜치 위치는 비교에 영향을 주지 않습니다.

리포트는 파일 통계와 diff 미리보기, 전체 텍스트 `changes.patch`를 제공합니다. 같은 파일을 여러 커밋에서 수정하면 최종 비교와 개별 커밋 내역이 다를 수 있습니다. 미리보기 크기와 바이너리 처리는 [코드 비교](README.ko.md#코드-비교)를 확인하세요.

`codeComparison.status`는 `AVAILABLE`, `NO_CHANGES`, `UNAVAILABLE`입니다. 반영한 커밋이 없으면 변경 없음으로 표시합니다. 비교 수집이나 patch 저장 실패는 사유만 기록하고 실행 결과와 종료 코드는 유지합니다.

과거 JSON에 `codeComparison`이 없으면 비교 미저장 안내를 표시합니다. `report --lang ko`는 새 diff 수집, 파일 변경, 모델 호출 없이 저장된 JSON을 출력합니다.

## 실패와 복구 분기

![거절하면 후보 수정을 복원한 뒤 재조사하며 안전 정지 시 worktree를 보존하고 리포트를 작성합니다.](../docs/assets/workflow/outcomes.ko.png)

[HTML 원본](../docs/assets/workflow/outcomes.ko.html) · [원본 흐름과 대응 기록](../docs/assets/workflow/README.md)

### 근거 부족과 알 수 없는 위험도

Audit의 `risk_level: "UNKNOWN"`은 `READY`여도 `RISK_UNKNOWN`으로 제외합니다. `NEEDS_EVIDENCE`는 deep check로 진행할 수 있습니다. Deep check가 `READY`를 반환하지 않으면 `NOT_READY`를 기록하고 수정 전에 후보를 멈춥니다.

<details>
<summary>전체 JSON 응답</summary>

<!-- example: deepcheck-evidence schema: deepcheck -->
```json
{
  "schema_version": "1",
  "candidate_id": "dead-legacy-parser",
  "category": "DEAD_CODE",
  "title": "Remove the unreferenced legacy parser",
  "minimal_change": "Delete src/legacy-parser.mjs.",
  "allowlist": [
    "src/legacy-parser.mjs"
  ],
  "forbidden_files": [
    "src/registry.mjs",
    "src/plugin-x.mjs"
  ],
  "contracts": [
    {
      "contract_id": "C1",
      "statement": "Existing src/index.mjs exports, return values and side effects stay unchanged.",
      "kind": "RETURN_SHAPE",
      "verified_by": "Inspect reachable imports and run the existing entrypoint tests and build."
    }
  ],
  "stop_conditions": [
    "Stop if an existing entrypoint or package consumer reaches the module.",
    "Stop if removal requires edits outside the allowlist."
  ],
  "rollback": {
    "strategy": "GIT_RESET_HARD",
    "detail": "The controller restores only the detached run worktree to its recorded pre-write commit."
  },
  "risk_level": "L0_LOW",
  "characterization_needed": false,
  "characterization_files": [],
  "readiness": "NEEDS_EVIDENCE",
  "readiness_reason": "The available configuration does not establish whether a loader reaches the module.",
  "notes": "Alternative branch, not part of the successful fixture example."
}
```

</details>

### Preflight 차단

가설이 반증되지 않거나 판단할 수 없으면 구현을 차단합니다. 예시는 오래된 근거로 로더 가설을 `INCONCLUSIVE`로 판단하고 `PREFLIGHT_BLOCKED`를 기록합니다. 앞서 반영한 characterization 커밋은 남을 수 있습니다.

<details>
<summary>전체 JSON 응답</summary>

<!-- example: preflight-blocked schema: preflight -->
```json
{
  "schema_version": "1",
  "candidate_id": "dead-legacy-parser",
  "verdict": "BLOCKED",
  "failure_hypothesis": "A string-based loader still resolves the legacy parser.",
  "falsification_method": "Inspect the registry, imports, scripts and configuration for the module path and exported symbols.",
  "falsification_result": "INCONCLUSIVE",
  "blocking_reasons": [
    {
      "code": "EVIDENCE_STALE",
      "detail": "The available registry evidence does not cover the current source snapshot.",
      "file": "src/registry.mjs"
    }
  ],
  "notes": "Alternative branch. No refactor should start with this result."
}
```

</details>

### Characterization만 반영되는 경우

작업 명세가 characterization을 요청하고 `policy.auto_characterization`이 켜져 있으면 기존 계약을 확인할 테스트를 추가합니다. 모델은 `sharpen-clarify`와 `sharpen-challenge`를 사용하고 `characterization_files`만 수정합니다.

아래 시나리오는 진입점 테스트를 요청합니다. 사용하지 않는 모듈을 관찰하려고 새 호출부를 만들면 안 됩니다.

<details>
<summary>전체 작업 명세 JSON</summary>

<!-- example: characterization-packet schema: deepcheck -->
```json
{
  "schema_version": "1",
  "candidate_id": "dead-legacy-parser",
  "category": "DEAD_CODE",
  "title": "Remove the unreferenced legacy parser",
  "minimal_change": "Delete src/legacy-parser.mjs.",
  "allowlist": [
    "src/legacy-parser.mjs"
  ],
  "forbidden_files": [
    "src/registry.mjs",
    "src/plugin-x.mjs"
  ],
  "contracts": [
    {
      "contract_id": "C1",
      "statement": "Existing src/index.mjs exports, return values and side effects stay unchanged.",
      "kind": "RETURN_SHAPE",
      "verified_by": "Inspect reachable imports and run the existing entrypoint tests and build."
    }
  ],
  "stop_conditions": [
    "Stop if an existing entrypoint or package consumer reaches the module.",
    "Stop if removal requires edits outside the allowlist."
  ],
  "rollback": {
    "strategy": "GIT_RESET_HARD",
    "detail": "The controller restores only the detached run worktree to its recorded pre-write commit."
  },
  "risk_level": "L0_LOW",
  "characterization_needed": true,
  "characterization_files": [
    "test/entrypoint-characterization.test.mjs"
  ],
  "readiness": "READY",
  "readiness_reason": "Reachability is established, but record the retained entrypoint return shape before deleting the module.",
  "notes": "Alternative test-protection scenario; the successful example skips characterization."
}
```

</details>

응답은 테스트 경로와 확인한 계약을 담습니다. 컨트롤러는 `PASS`여도 운영 코드 수정, assertion 약화와 범위 밖 변경을 거절합니다.

운영 코드를 유지한 채 테스트를 실행하고 별도 characterization 커밋을 만듭니다. 빈 diff는 커밋하지 않으며 테스트 경로 누락이나 검증 실패는 후보를 중단합니다. 자동 characterization을 끄면 테스트 작성 없이 preflight로 진행합니다.

<details>
<summary>전체 JSON 응답</summary>

<!-- example: characterization-success schema: characterization -->
```json
{
  "schema_version": "1",
  "candidate_id": "dead-legacy-parser",
  "changed_files": [
    "test/entrypoint-characterization.test.mjs"
  ],
  "characterized_contracts": [
    "C1"
  ],
  "production_source_changed": false,
  "assertions_weakened": false,
  "verdict": "PASS",
  "notes": "Illustrative response. The controller must still validate the actual test diff and run it against unchanged production code."
}
```

</details>

이후 preflight 차단, 검증 회귀나 리뷰 거절 시에는 **characterization 이후 커밋**으로 복원합니다. 테스트 커밋은 결과 브랜치에 남으므로 리팩터링 커밋이 0개여도 최종 비교에 테스트가 포함될 수 있습니다. `policy.max_commits`는 리팩터링 커밋만 셉니다.

### 검증 회귀와 리뷰 거절

기존에 통과한 명령은 계속 통과해야 합니다. 해석 가능한 기존 실패에 새 오류 signature가 생기거나 타임아웃, 실행 불가가 발생하면 후보를 거절합니다. 같은 실패도 통과로 세지 않습니다. 컨트롤러는 결과와 실패를 기록하고 수정 직전 커밋으로 복원합니다.

아래 리뷰 예시는 동작 보존을 확인하지 못해 리팩터링을 되돌립니다. 저장한 `accepted.patch`는 입력 스냅샷으로 남으며 승인을 뜻하지 않습니다.

<details>
<summary>전체 JSON 응답</summary>

<!-- example: review-rejected schema: review -->
```json
{
  "schema_version": "1",
  "candidate_id": "dead-legacy-parser",
  "verdict": "FAIL",
  "behavior_preservation_assessment": "CANNOT_DETERMINE",
  "unreviewable_hunks": [
    "H1"
  ],
  "findings": [
    {
      "finding_id": "R1",
      "severity": "BLOCKER",
      "file": "src/legacy-parser.mjs",
      "locator": "H1",
      "claim": "The reviewed evidence does not rule out a configured loader.",
      "why_it_matters": "Removing a reachable module would change an existing entrypoint.",
      "suggested_action": "Retain the module until the loader configuration is checked."
    }
  ],
  "notes": "Alternative branch illustrating review rejection, not a finding from the fixture."
}
```

</details>

### 스키마 복구, 타임아웃과 프로바이더 전환

| 상황 | 컨트롤러 처리 | 다음 단계 |
| --- | --- | --- |
| JSON 형식이나 스키마 오류 | 검증 오류와 이전 응답을 넣어 읽기 전용 복구를 한 번 요청 | 다시 검사하고 `SCHEMA`가 남으면 사용 가능한 다른 프로바이더에 요청 |
| `TIMEOUT` 또는 `PROCESS` 실패 | 같은 프로바이더에 5초, 그다음 20초 뒤 재시도 | 재시도를 소진하면 사용 가능한 다른 프로바이더에 요청 |
| `QUOTA` 또는 `AUTH` | 프로바이더 상태를 갱신하고 handoff 스냅샷 작성 시도 | 같은 단계에서 다른 프로바이더에 요청 |
| 치명적인 프로바이더 오류 | 복구나 전환 없이 중단 | 진단 기록을 보존하고 종료 사유 표시 |
| 해당 단계를 완료한 프로바이더 없음 | 단계별 실패 또는 프로바이더 소진 처리 | 컨트롤러 상태와 한도에 따라 실패를 기록하거나 종료 |

복구는 원래 응답 스키마를 사용하며 별도 JSON 형식이 없습니다. 복구 세션에서는 코드 수정을 이어갈 수 없습니다. 프로세스가 성공해도 스키마 검사를 통과해야 진행합니다. 실패와 복구 프로세스도 사용량에 포함합니다.

전환 시 해당 단계의 입력으로 새 세션을 시작하며 이전 세션을 재개하지 않습니다. 쓰기 단계의 `callPhase`는 재시도나 전환 직전에 매번 복원하지 않습니다. 실행 실패나 검사 거절에 따른 복원은 후보 처리 코드가 담당합니다.

`handoff.md`는 사람이 읽는 중간 기록입니다. 다음 프로바이더의 대화 문맥이나 최종 리포트로 사용하지 않습니다.

## 확인할 기록

| 파일 | 의미 |
| --- | --- |
| `audits/*.json` | Audit이 반환한 후보 목록 |
| `cycles/*/packet.json` | 작업 명세와 컨트롤러 메타데이터 |
| `cycles/*/preflight.json` | 실패 가설과 반증 결과 |
| `cycles/*/execution.json` | 구현 응답과 프로바이더 |
| `cycles/*/gate.json`, `validation.json`, `review.json` | 해당 사이클의 범위 검사, 실행한 검증과 리뷰 근거 |
| `cycles/*/accepted.patch` | 리뷰에 제출한 diff. 나중에 거절한 후보에도 남을 수 있음 |
| `state.json` | 종료 상태, 카운터와 최종 반영 커밋 OID |
| `report.json`, `report.md`, `changes.patch` | 최종 리포트와 커밋된 텍스트 비교. 비교 불가 시 사유 기록 |

경로는 `.refactor/runs/<id>/` 기준이며 사이클 디렉터리명에 유형과 fingerprint가 포함됩니다. [상태·비용·종료 코드](README.ko.md#사용량과-종료-코드)와 [병합 전 검토](TUTORIAL.ko.md#결과-확인)를 확인하세요. 로컬 테스트는 스키마와 컨트롤러 동작을 검사하며 실제 모델의 판단 품질은 측정하지 않습니다.
