# 2026-11-19 전체 코드 검토 및 하단 상태바 에이전트 잔여 사용량 표시 결정

## 1. 검토 배경 및 발견된 문제

### 1) 새 맥 설치 시 1회차 실패 → 재실행 필요 원인
- 이 머신의 설치 타임스탬프 대조(2026-11-01 `14:22` ~ `14:48`) 결과, `iterm2_zsh/install.sh`와 `pier` README의 Homebrew 설치 명령이 `brew tap zkzofn/tap` 없이 `brew install zkzofn/tap/pier` 단일 호출로 auto-tap에 의존하고 있었음.
- 1회차(`14:27:10`)에서 tap 저장소만 클론되고 formula 인식에 실패해 중단되었다가, 2회차(`14:47:58`)에서 기존 tap을 읽어 설치에 성공함.
- 추가 위험 요소:
  - `main.go`의 `self()`가 `os.Executable()` 결과를 그대로 사용해 Homebrew 심링크가 풀린 버전 고정 Cellar 경로(`/opt/homebrew/Cellar/pier/<ver>/bin/pier`)가 설정에 기록될 위험.
  - `tmux.Attach()`가 `exec.ExitError`를 `return nil`로 삼켜 실패 원인이 감춰짐.

### 2) 코드/구조/UX 결함
- **`setup.Run()`에서 `AgyHooks()` 미호출 버그**: `AgyHooks` 함수와 테스트가 구현되어 있으나 `setup.Run()` 및 `NeedsSetup()`에 연결되지 않아 `~/.gemini/config/hooks.json`이 생성되지 않고, 실행 중인 `agy` 세션 상태가 전부 `·`로 고정됨.
- **커스텀 에이전트 인식 누락**: `discover` 패키지가 내장 프리셋(`Builtins`)만 인식하고 `config.json`의 `agents` 오버라이드를 반영하지 못함.
- **`EnsureSidebar` 동시성**: `client-attached`와 `client-session-changed`가 동시에 실행될 때 중복 사이드바 생성 방지 락 부재.
- **사이드바 스크롤 및 에이전트 구분**: 세션 수가 터미널 높이를 넘을 때 스크롤이 없어 하단 액션(`+ new session`, `? help`)이 잘리고 클릭 Y좌표가 어긋남. 복수 에이전트 동시 실행 시 구분 배지 부재.
- **피커 UX**: 설치된 에이전트가 1개뿐일 때도 `[<agent> · ^A]`와 `^A: agent` 힌트가 노출됨.
- **`agy` 대화 제목 폴백**: `ui.sessionTitle()`이 `~/.claude/projects/`만 조회함.
- **`iterm2_zsh` 드리프트**: `brew tap` 누락, 폐기된 `cl` alias 강제 실행(온보딩 위저드 스킵 유발), `~/.zshrc` 변경(`alias agy=...`) 미반영 등은 `pier` 작업 완료 후 별도 후속 작업으로 정리 예정.

## 2. 의사결정 사항

### 1) Gemini CLI 완전 제외 확정
- `gemini` CLI는 `antigravity`(`agy`)로 완전히 대체되었으므로 지원 대상에서 제외 유지(내장 프리셋: `claude`, `agy`, `codex`, `grok`).

### 2) 하단 상태바 에이전트 잔여 사용량 표시 — **안 A 채택**
- **표시 범위**: 현재 보고 있는 tmux 세션의 활성 대표 에이전트(`discover.RepresentativeWithCustom`) 잔여 사용량만 우측 상태바(`status-right`)에 표시. 일반 터미널(`$`) 세션에서는 생략해 가로 폭(기본 120 cols 등)을 효율적으로 사용.
- **호출 계약**: `pier status "#{pane_current_path}" "#{session_name}" "#{pane_id}" "#{client_width}"` (기존 1인자 호출 하위 호환 유지).
- **데이터 수집 방식 (비공식 API 폴링 배제)**:
  - **Antigravity (`agy`)**: `~/.gemini/antigravity-cli/settings.json`에 `statusLine`으로 `pier statusline agy`(`stack_with_default: true`)를 연결해 `quota` JSON(`remaining_fraction`, `reset_time`, `reset_in_seconds`)을 `~/.local/state/pier/usage/agy.json`에 로컬 캐시.
  - **Claude Code (`claude`)**: `~/.claude/settings.json`의 기존 `statusLine`을 `pier statusline claude --wrap '...'` 투명 래퍼로 감싸 `rate_limits.five_hour` / `rate_limits.seven_day`의 잔여량(`100 - used_percentage`)을 `~/.local/state/pier/usage/claude.json`에 캐시하면서 기존 statusLine 출력은 그대로 유지.
  - **Codex (`codex`)**: `~/.codex/sessions/**/rollout-*.jsonl`의 마지막 `rate_limits`를 방어적으로 파싱.
  - **Grok (`grok`)**: 5h/주간 구독 잔여량 개념이 없으므로 표시 생략.
- **외부 설정 파일 자동 연동 승인**: `pier setup`에서 `~/.tmux.conf`, `~/.claude/settings.json`(hooks + statusLine wrap), `~/.gemini/config/hooks.json`, `~/.gemini/antigravity-cli/settings.json`(statusLine) 자동 구성 승인.
