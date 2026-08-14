# Pier

[English](README.md)

**tmux 안에서 쓰는 Claude Code 세션 대시보드.** 세션들이 정박하는 부두(pier).
지금 어떤 Claude Code 세션들이 돌고 있는지, 어느 워크트리·브랜치에서, 어떤 상태(작업중/입력대기)인지 사이드바로 보여주고, 클릭 한 번으로 그 세션에 점프한다. 쓰던 tmux 워크플로를 그대로 유지하면서, 사이드바 하나당 **8MB**로.

```
┌──────────────────────┬─────────────────────────────────────┐
│ Pier — sessions      │                                     │
│                      │                                     │
│ suite2 ⎇ feat/nmt    │                                     │
│  ● 결제 모듈 리팩토링  │        Claude Code (main pane)      │
│ suite3 ⎇ dev         │                                     │
│  ○ suite3            │                                     │
│ terminal ⎇ -         │                                     │
│  $ terminal        ◀ │                                     │
└──────────────────────┴─────────────────────────────────────┘
 [session]                      [~/dev/suite2 ⎇ feat/nmt 13:53]
```

- **사이드바**: 실행 중인 모든 Claude Code 인스턴스를 워크트리(경로+브랜치)별로 그룹핑. 항목 이름은 대화의 AI 생성 제목(없으면 tmux 세션명). CC pane이 하나도 없는 세션도 `$`로 함께 표시 — 일반 셸 세션도 목록에서 보이고 점프된다
- **상태 아이콘**: `●` 작업중 / `○` 입력 대기 / `!` 권한 승인 대기 / `·` 미확인 / `$` 일반 셸 세션
- **점프**: 항목을 마우스로 클릭 → 다른 tmux 세션이어도 바로 전환
- **status bar**: 우측에 현재 pane의 경로 + git 브랜치 상시 표시 (detached HEAD는 `detached@sha`)
- **자동 부착**: 세션에 attach하거나 전환하면 사이드바가 알아서 생긴다
- **새 세션 생성**: `+ new session` 클릭, 사이드바에서 `n`, 또는 아무 데서나 `prefix+N` — 중앙 popup에서 디렉토리를 고르면 그 이름의 Claude Code 세션이 그 경로에서 시작된다. `Enter` 대신 `^S`를 누르면 일반 셸 세션 — pane 분할이나 새 윈도우 없이 터미널이 필요할 때. tmux 밖 일반 터미널에서도 `pier new`로 같은 picker가 뜨고, 만든 세션에 그대로 attach된다
- **복구(resume)**: 크래시·전원 차단·OS 재부팅으로 날아간 Claude Code 세션은 사라지지 않는다 — picker에서 그 디렉토리에 `↻ resume` 버튼이 붙는다. `→` 후 `Enter`면 그 대화를 기존 세션명으로 이어가고, 그냥 `Enter`면 빈 세션으로 시작하며 복구 기회는 남는다. 사상자가 있으면 맨 위 `↻ restore all` 행이 셧다운 전 세션 구성 전체를 한 번에 되살린다
- **텔레그램**: picker에서 `^T`(켜지면 `tg✓` 배지) — Claude Code의 텔레그램 채널을 붙인 채로 세션을 시작한다
- **도움말**: 사이드바 하단 `? help` 클릭(또는 `?`) — 위의 모든 키를 popup으로 보여주니 외울 필요 없다

## 요구사항

- [Claude Code](https://claude.com/claude-code) — 이 대시보드가 보여주는 대상
- tmux 3.2+ (3.6a에서 개발·검증; Homebrew 설치 시 자동으로 함께 설치됨)
- Go 1.24+는 소스 빌드 시에만 — Homebrew는 프리빌트 바이너리 제공
- macOS에서 개발·검증. Linux는 이론상 동작하나 미검증

## 설치

### Homebrew

```sh
brew install zkzofn/tap/pier
pier setup
```

### 소스 빌드

```sh
git clone https://github.com/zkzofn/pier.git
cd pier
make setup          # 빌드 + ~/.local/bin 설치 + pier setup
```

`pier setup`은 `~/.tmux.conf`에 마커 블록을 추가하고 `~/.claude/settings.json`에
hook 6종을 병합한다 — 기존 설정은 보존되고 `.bak-pier` 백업을 먼저 남긴다.
재실행해도 중복되지 않으며, tmux 서버가 떠 있으면 즉시 리로드까지 한다.

<details>
<summary>수동 설정 — <code>pier setup</code>이 하는 일</summary>

### 1. tmux 설정

`~/.tmux.conf`에 추가:

```tmux
set -g mouse on
set -g status-interval 5
set -g status-right-length 60
set -g status-right '#(~/.local/bin/pier status "#{pane_current_path}") %H:%M '

# attach·세션 전환 시 사이드바 자동 생성 (이미 있으면 no-op)
set-hook -g client-attached 'run-shell "~/.local/bin/pier ensure"'
set-hook -g client-session-changed 'run-shell "~/.local/bin/pier ensure"'

# prefix + g : 사이드바 토글
bind-key g run-shell "~/.local/bin/pier toggle"

# prefix + x : 현재 세션 종료 후 다음 세션으로 (사이드바 순서)
# (기본 kill-pane 바인딩을 덮어씀 — pane 닫기는 exit/Ctrl-D로)
bind-key x run-shell "~/.local/bin/pier done"

# prefix + N : 새 세션 picker 열기
bind-key N display-popup -E -w 46 -h 18 -T " New session " "~/.local/bin/pier new"
```

적용: `tmux source-file ~/.tmux.conf`

### 2. Claude Code hooks (상태 아이콘용)

`~/.claude/settings.json`의 `hooks`에 추가 (경로의 `<you>`를 수정):

```json
{
  "hooks": {
    "UserPromptSubmit": [{ "hooks": [{ "type": "command", "command": "/Users/<you>/.local/bin/pier hook user-prompt-submit", "timeout": 5 }] }],
    "PreToolUse": [{ "matcher": "*", "hooks": [{ "type": "command", "command": "/Users/<you>/.local/bin/pier hook pre-tool-use", "timeout": 5 }] }],
    "Stop": [{ "hooks": [{ "type": "command", "command": "/Users/<you>/.local/bin/pier hook stop", "timeout": 5 }] }],
    "PermissionRequest": [{ "matcher": "*", "hooks": [{ "type": "command", "command": "/Users/<you>/.local/bin/pier hook permission-request", "timeout": 5 }] }],
    "SessionStart": [{ "hooks": [{ "type": "command", "command": "/Users/<you>/.local/bin/pier hook session-start", "timeout": 5 }] }],
    "SessionEnd": [{ "hooks": [{ "type": "command", "command": "/Users/<you>/.local/bin/pier hook session-end", "timeout": 5 }] }]
  }
}
```

hooks 없이도 사이드바·점프·status bar는 전부 동작한다. 상태 아이콘(`·`로 고정)과 프롬프트 라벨, 크래시 복구만 빠진다.

</details>

## 사용법

| 하고 싶은 것 | 방법 |
|---|---|
| 다른 세션으로 점프 | 사이드바 항목 **클릭** |
| 새 세션 생성 | `+ new session` 클릭(사이드바 포커스에서 `n`, 아무 데서나 `prefix+N`, tmux 밖에서는 `pier new`). 타이핑으로 디렉토리 필터(`~/dev/*` + 과거 CC 사용 경로), `Enter` 생성, `^S`는 Claude Code 대신 일반 셸로 생성, `^T`는 텔레그램 채널 부착, `Tab`으로 제안된 이름 수정. 없는 경로를 입력하면 `mkdir & create`로 디렉토리까지 만들어 진행, 이미 열린 경로를 고르면 생성 대신 점프 |
| 죽은 세션 복구 | picker에서 크래시·셧다운으로 끊긴 대화가 있는 디렉토리엔 `↻ resume`이 표시된다: `→` 후 `Enter`면 이어서, 그냥 `Enter`면 빈 세션(복구 기회는 보존). 맨 위 `↻ restore all` 행(`↑` 한 번)은 사상자 전부를 각자의 세션으로 되살린다 |
| 키보드로 점프 | 사이드바 pane으로 포커스 이동(`prefix+←`) 후 `j`/`k` + `Enter` |
| 단축키 치트시트 | 사이드바 하단 `? help` 클릭, 또는 사이드바 포커스에서 `?` — 아무 키나 누르면 닫힘 |
| 사이드바 열기/닫기 | `prefix + g`, 또는 `pier toggle` |
| 현재 세션 끝내기 | `prefix + x` (또는 `pier done`) — 세션을 종료하고 사이드바 순서상 다음 세션으로 점프. 다른 세션이 없으면 그냥 종료. 확인 절차 없이 즉시 kill. tmux 기본 kill-pane 바인딩을 덮어씀 |
| 강제 새로고침 | 사이드바에서 `r` |
| 사이드바 종료 | 사이드바에서 `q` |

키 입력은 tmux가 포커스된 pane으로 보내므로, `j`/`k`는 사이드바에 포커스가 있을 때만 동작한다. 일상 사용은 클릭이 기본이다.

## 동작 원리

```
┌─ tmux 세션 (세션마다 사이드바 1개) ─────────────┐
│ ┌────────┐  ┌──────────────────────────────┐  │
│ │  pier  │  │  Claude Code                 │  │
│ │  run   │  │                              │  │
│ └───┬────┘  └──────────────┬───────────────┘  │
└─────┼──────────────────────┼──────────────────┘
      │ 2s 폴링:              │ hooks
      │ tmux list-panes -a   ▼
      │◀─ fsnotify ── ~/.local/state/pier/panes/<pane>.json
```

- **진실의 원천은 tmux.** 2초마다 `list-panes -a`를 폴링해 Claude Code pane을 판별한다(프로세스명이 `claude` 또는 버전 문자열 `2.1.206` 형태). 상태 파일은 장식일 뿐이라 hook이 죽어도 목록은 항상 정확하다.
- **상태는 CC hooks가 기록.** `pier hook <event>`는 CC의 자식 프로세스로 실행되어 `$TMUX_PANE`을 상속받으므로 pane과 정확히 매핑된다. TUI는 fsnotify로 즉시 반영한다.
- **항목 라벨 = 현재 프롬프트.** 각 인스턴스 행에는 그 pane에서 마지막으로 제출한 프롬프트가 표시된다(`UserPromptSubmit` hook payload에서 캡처). `/clear`는 새 세션을 시작하므로(`SessionStart` hook) 다음 프롬프트 전까지 라벨이 공백이 된다. 아직 기록된 프롬프트가 없으면(설치 직후, resume) `~/.claude/projects/*/<id>.jsonl`의 `ai-title` 레코드 → tmux 세션명 순으로 폴백한다.
- **크래시·셧다운 복구.** `UserPromptSubmit`마다 생존 마커(`~/.claude/live-sessions/<session-id>.json`, 세션의 cwd·pid·tmux 세션명 포함)를 남기고, `SessionEnd`가 이를 종료 로그로 회수한다. picker는 다음 두 경우에 그 디렉토리를 사상자 보유로 표시한다:
  - *크래시* (SIGKILL, 커널 패닉, 전원 차단): hook이 돌지 못했으므로 마커가 죽은 pid를 담은 채 남아 있다.
  - *정상 OS 셧다운*: hook이 돌아 마커는 사라진다 — 대신 사이드바가 1분마다 touch하는 하트비트 파일이 이전 부팅에서 동결된 시각 ±90초 안에 끝난 세션을 셧다운 사상자로 판정한다. 사용자가 의도한 종료(`/clear`, logout, 프롬프트에서 exit)는 제외된다.

  복구는 언제나 명시적 선택이다 — `↻ resume` 버튼 또는 `↻ restore all`이며, 자동으로 이어붙이지 않는다. 실제로 resume했을 때만 기록이 소비되고, 빈 세션으로 시작하면 남는다. 기록은 7일이 지나거나 대화 트랜스크립트가 사라지면 만료된다. 한계: 셧다운 감지는 셧다운 당시 사이드바가 떠 있었어야 하고(하트비트의 주인), 셧다운 직전 ~90초 안에 직접 닫은 세션은 오탐으로 표시될 수 있다 — 무시해도 되는 제안일 뿐이다.

## 메모리 사용량 (실측)

macOS(Apple Silicon), RSS 기준. 사이드바는 tmux 구조상 세션마다 1개씩 뜬다.

| 항목 | 측정값 |
|---|---|
| 사이드바 1개 | 7.1 ~ 9.7 MB (평균 8.1 MB) |
| 세션 6개 운영 시 합계 | 48.8 MB |
| `pier hook` / `pier status` | 상주 없음 (이벤트당 수 ms 단발 실행) |
| 바이너리 크기 | 5 MB |

## 트러블슈팅

- **사이드바가 죽어 있음** → `prefix + g` 두 번 (제거 후 재생성)
- **discover가 뭘 보는지 확인** → `go run ./cmd/dbg`
- **마우스 텍스트 선택이 안 됨** → `mouse on`의 트레이드오프. iTerm2는 `Option+드래그`로 네이티브 선택
- **상태 아이콘이 전부 `·`** → hooks 미설정이거나, 떠 있던 CC 세션이 아직 새 프롬프트를 받지 않은 상태. 다음 프롬프트부터 기록된다

## 라이선스

MIT
