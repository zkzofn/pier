# Pier

**tmux 안에서 쓰는 Claude Code 세션 대시보드.** 세션들이 정박하는 부두(pier).
지금 어떤 Claude Code 세션들이 돌고 있는지, 어느 워크트리·브랜치에서, 어떤 상태(작업중/입력대기)인지 사이드바로 보여주고, 클릭 한 번으로 그 세션에 점프한다. 쓰던 tmux 워크플로를 그대로 유지하면서, 사이드바 하나당 **8MB**로.

```
┌──────────────────────┬─────────────────────────────────────┐
│ Pier — CC sessions   │                                     │
│                      │                                     │
│ suite2 ⎇ feat/nmt    │                                     │
│  ● 결제 모듈 리팩토링  │        Claude Code (main pane)      │
│ suite3 ⎇ dev         │                                     │
│  ○ suite3            │                                     │
│ terminal ⎇ -         │                                     │
│  ● Tmux 대시보드 개발 ◀│                                     │
└──────────────────────┴─────────────────────────────────────┘
 [session]                      [~/dev/suite2 ⎇ feat/nmt 13:53]
```

- **사이드바**: 실행 중인 모든 Claude Code 인스턴스를 워크트리(경로+브랜치)별로 그룹핑. 항목 이름은 대화의 AI 생성 제목(없으면 tmux 세션명)
- **상태 아이콘**: `●` 작업중 / `○` 입력 대기 / `!` 권한 승인 대기 / `·` 미확인
- **점프**: 항목을 마우스로 클릭 → 다른 tmux 세션이어도 바로 전환
- **status bar**: 우측에 현재 pane의 경로 + git 브랜치 상시 표시 (detached HEAD는 `detached@sha`)
- **자동 부착**: 세션에 attach하거나 전환하면 사이드바가 알아서 생긴다

## 요구사항

- tmux 3.2+ (3.6a에서 개발·검증)
- Go 1.24+ (빌드용)
- macOS에서 개발·검증. Linux는 이론상 동작하나 미검증

## 설치

```sh
git clone https://github.com/zkzofn/pier.git
cd pier
make install        # → ~/.local/bin/pier
```

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
    "PermissionRequest": [{ "matcher": "*", "hooks": [{ "type": "command", "command": "/Users/<you>/.local/bin/pier hook permission-request", "timeout": 5 }] }]
  }
}
```

hooks 없이도 사이드바·점프·status bar는 전부 동작한다. 상태 아이콘만 `·`(미확인)으로 남는다.

## 사용법

| 하고 싶은 것 | 방법 |
|---|---|
| 다른 세션으로 점프 | 사이드바 항목 **클릭** |
| 키보드로 점프 | 사이드바 pane으로 포커스 이동(`prefix+←`) 후 `j`/`k` + `Enter` |
| 사이드바 열기/닫기 | `prefix + g`, 또는 `pier toggle` |
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
- **대화 제목**: hook payload의 session id로 `~/.claude/projects/*/<id>.jsonl`의 `ai-title` 레코드를 읽는다.

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
