# 2026-11-02 멀티 AI CLI 플랫폼 진입 분기 및 연동 설계

## 배경
기존 `pier`는 Claude Code(`claude`) 전용으로 하드코딩되어 있었으며, 첫 실행 시 `alias cl='pier'`를 유도하는 구조였다.
Antigravity(`agy`), OpenAI Codex(`codex`), Grok(`grok`) 등 복수의 AI CLI 플랫폼을 선택·전환하여 사용할 수 있도록 진입 분기와 설정 구조를 개편한다.

## 주요 의사결정 이력

### 1. 진입 및 전환 UX
- **결정**: `~/.config/pier/config.json`에 기본 에이전트(`agent`)를 저장하고, 세션 생성 피커(`pier new` / `prefix+N`)에서 `^A`(`ctrl+a`) 단축키로 세션별 에이전트를 순환 전환한다.
- **갱신 (2026-11-02)**:
  - 피커에서 `^A`가 순환 전환 단축키임을 사용자가 즉시 인지할 수 있도록 상단 입력줄 우측에 `[<agent> · ^A]` 배지를 표시하고, 하단 단축키 안내줄에도 `^A: switch agent`를 명시적으로 노출한다.
  - 기존 `cl` 쉘 별칭(alias) 중심 실행 대신 `pier` 명령어로 직접 실행하도록 온보딩 위저드에서 `cl` alias 생성 유도 단계를 제거한다.

### 2. 지원 플랫폼(프리셋) 목록 관리
- **초안**: `claude`, `agy`, `gemini`, `codex`, `grok` 중 `PATH`에 설치된 바이너리 자동 노출 + `config.json` 커스텀 CLI 지원.
- **갱신 (2026-11-02)**: `gemini` CLI는 Antigravity(`agy`)로 대체되었으므로 내장 프리셋에서 제외하고 **`claude`, `agy`, `codex`, `grok` 4종**만 내장 프리셋으로 유지한다.

### 3. 플랫폼별 부가 연동 범위
- **결정**:
  - `discover`: `claude` 외에 `agy`, `codex`, `grok`도 에이전트 패인으로 인식하여 사이드바에서 일반 쉘(`$`)과 구분.
  - `setup` / `state`: Antigravity(`agy`)는 `~/.gemini/config/hooks.json`을 통해 `SessionStart`, `PreInvocation`, `PostToolUse`, `Stop` 생명주기 훅을 지원하므로, `pier setup` 시 자동 연결하고 camelCase 페이로드(`conversationId`, `workspacePaths`, `lastUserInput`)를 파싱하여 사이드바 실시간 상태(`●`/`○`) 및 프롬프트 라벨을 표시한다.
  - `newsess`: `~/.gemini/antigravity-cli/history.jsonl`의 `workspace` 경로도 후보 디렉토리로 수집한다.
