# Pier

[English](README.md)

**tmux 안에서 쓰는 AI 코딩 세션 사이드바 (`claude`, `agy`, `codex`, `grok`).** 세션들이 정박하는 부두(pier).
지금 어떤 AI 세션들이 돌고 있는지, 어느 워크트리·브랜치에서, 어떤 상태(작업중/입력대기)인지 사이드바로 보여주고, 클릭 한 번으로 그 세션에 점프한다. 쓰던 tmux 워크플로를 그대로 유지하면서, 사이드바 하나당 **15MB**로.

```
+----------------------+-------------------------------------+
| Pier - sessions      |                                     |
|                      |                                     |
| web (feat/search)    |                                     |
|  * Refactor payments |       AI agent (main pane)          |
| api (dev)            |                                     |
|  o api               |                                     |
| terminal (-)         |                                     |
|  $ terminal        < |                                     |
+----------------------+-------------------------------------+
 [session]    [claude 5h:82% | ~/dev/web (feat/search) 13:53]
```

- **사이드바**: 실행 중인 모든 AI CLI 인스턴스(`claude`, `agy`, `codex`, `grok` 및 `~/.config/pier/config.json`의 커스텀 `agents`)를 워크트리(경로+브랜치)별로 그룹핑. 항목 이름은 현재 프롬프트 또는 대화 제목(없으면 tmux 세션명). 여러 종류의 에이전트가 함께 돌면 행 앞에 짧은 구분 배지(`cl:`, `agy:`, `cdx:`, `grk:`)가 붙는다. 목록이 길어지면 마우스 휠이나 `j`/`k`로 스크롤. 에이전트 pane이 하나도 없는 세션도 `$`로 함께 표시 — 터미널 세션도 목록에서 보이고 점프된다
- **상태 아이콘**: `●` 작업중 / `○` 입력 대기 / `!` 권한 승인 대기 / `·` 미확인 / `$` 터미널 세션(일반 셸)
- **점프**: 항목을 마우스로 클릭 → 다른 tmux 세션이어도 바로 전환
- **status bar**: 우측에 현재 보고 있는 세션의 활성 에이전트 잔여 사용량(`claude` `5h`/`7d`, `agy` `5h`/`7d`/`Argon`, `codex` `5h`/`7d`)과 현재 pane의 경로 + git 브랜치 상시 표시 (detached HEAD는 `detached@sha`)
- **자동 부착**: 세션에 attach하거나 전환하면 사이드바가 알아서 생긴다
- **`pier`가 입구**: tmux 밖에서는 picker가 뜨고 고른 세션에 그대로 attach, tmux 안에서는 설정된 기본 에이전트(`pier agent`)가 현재 pane에서 실행된다. `pier -r` 같은 인자는 그대로 기본 에이전트에 전달. 최초 실행 때 tmux·에이전트 hooks(`~/.claude/settings.json`, `~/.gemini/config/hooks.json`)·`statusLine` 사용량 캐시 배선과 기본 에이전트 선택을 진행하고, 이미 `config.json`이 있지만 `agent`가 비어 있는 기존 사용자에게도 다음 `pier` 실행 때 1회 기본 에이전트 선택을 제안한다. 하루에 한 번 Homebrew tap에 새 버전이 있는지 확인해 업그레이드를 제안한다
- **새 세션 생성**: `+ new session` 클릭, 사이드바에서 `n`, 또는 아무 데서나 `prefix+N` — picker는 실행 중인 세션을 맨 위에 먼저 보여주고(지금 있는 세션이 `▌` 표시로 가장 위), 그 아래 디렉토리 목록(`~/dev/*`, 과거 Claude Code 경로, Antigravity 기록)이 온다. picker는 만들기만 하고 전환은 하지 않는다(전환은 사이드바 몫): `^A`로 설치된 에이전트(`claude`, `agy`, `codex`, `grok`, 커스텀 `agents`)를 순환 전환하고, 실행 중 세션 행에서 `Enter`는 그 세션의 디렉토리에 에이전트 세션을 하나 더, 디렉토리 행에서 `Enter`는 그 경로에 새로 — 이름은 디렉토리를 따르고, 이미 있으면 `-2`, `-3`, … 이 붙는다. `Enter` 대신 `^S`를 누르면 그 행에 터미널 세션(일반 셸) — pane 분할이나 새 윈도우 없이 터미널이 필요할 때. picker의 키는 popup이 떠 있는 동안 그대로 누른다(prefix 불필요). 모든 키는 `▸` 커서가 가리키는 행에 작용하며, 커서는 현재 세션에서 시작하므로 `prefix+N` `Enter`가 곧 "여기에 에이전트 하나 더"다. tmux 밖 일반 터미널에서는 그냥 `pier`로 같은 picker가 뜨고, 거기서는 실행 중 세션 행의 `Enter`가 그 세션에 attach한다
- **복구(resume)**: 크래시·전원 차단·OS 재부팅으로 날아간 Claude Code 세션은 사라지지 않는다 — picker에서 그 디렉토리에 `↻ resume` 버튼이 붙는다. `→` 후 `Enter`면 그 대화를 기존 세션명으로 이어가고, 그냥 `Enter`면 빈 세션으로 시작하며 복구 기회는 남는다. 사상자가 있으면 맨 위 `↻ restore all` 행이 셧다운 전 세션 구성 전체를 한 번에 되살린다
- **텔레그램**: picker에서 `^T`(Claude Code 전용; 켜지면 `telegram ✓` 배지) — Claude Code의 텔레그램 채널을 붙인 채로 세션을 시작한다. `^T`는 먼저 플러그인이 켜져 있고 봇 토큰이 있는지 확인하고, 없으면 뭐가 빠졌는지 알려준다(`/plugin`, `/telegram:configure`) — Claude Code는 그냥 시작하고 채널만 조용히 안 붙기 때문. 플러그인 쪽 제약(0.0.7): 봇은 한 번에 한 Claude Code 세션만 잡을 수 있고, 플러그인이 켜진 새 Claude Code 세션이 열리면 그쪽이 가져간다
- **업데이트 알림**: Homebrew tap에 새 버전이 있으면 모든 사이드바 하단에 `↑ <버전> · pier upgrade` 줄이 뜬다
- **도움말**: 사이드바 하단 `? help` 클릭(또는 `?`) — 위의 모든 키를 popup으로 보여주니 외울 필요 없다

## 요구사항

- 지원하는 AI 코딩 CLI 중 1개 이상 ([Claude Code](https://claude.com/claude-code), [Antigravity CLI](https://antigravity.google) `agy`, `codex`, `grok`)
- tmux 3.2+ (3.6a에서 개발·검증; Homebrew 설치 시 자동으로 함께 설치됨)
- Go 1.24+는 소스 빌드 시에만 — Homebrew는 프리빌트 바이너리 제공
- macOS에서 개발·검증. Linux는 이론상 동작하나 미검증

## 설치

### Homebrew

```sh
brew tap zkzofn/tap
brew install zkzofn/tap/pier
pier
```

### 소스 빌드

```sh
git clone https://github.com/zkzofn/pier.git
cd pier
make setup          # 빌드 + ~/.local/bin 설치 + pier setup
pier
```

최초 `pier` 실행은 짧은 마법사다.

먼저 tmux·에이전트 hooks·상태바 사용량 연동을 제안한다 — `pier setup`과 같은 일로:
- `~/.tmux.conf`에 마커 블록 추가
- `~/.claude/settings.json`에 hook 6종 병합, `agy` 또는 `~/.gemini`가 있으면 `~/.gemini/config/hooks.json`에 hook 5종 병합
- `~/.claude/settings.json`(기존 `statusLine` 스크립트가 있으면 투명 래핑) 및 `~/.gemini/antigravity-cli/settings.json`에 `pier statusline`을 연결해 tmux 하단 상태바에서 현재 세션 에이전트의 5시간/주간 잔여 사용량을 표시

기존 설정은 보존되고 수정 전 `.bak-pier` 백업을 남기며, 재실행해도 중복되지 않고 tmux 서버가 떠 있으면 즉시 리로드한다. 참고로 `claude`와 `agy`는 시작 시점에만 `statusLine` 설정을 읽으므로, 이미 떠 있는 세션은 재시작해야 상태바 사용량이 갱신된다.

여러 에이전트가 설치되어 있으면 기본 에이전트를 선택한다(`pier agent <이름>`으로 언제든 변경 가능). 1.4.0 이전 버전에서 업그레이드해 `~/.config/pier/config.json`에 `agent` 항목이 아직 없는 경우에도 다음 `pier` 실행 때 1회 기본 에이전트 선택 프롬프트가 뜨며, 기존 `alias`나 커스텀 `agents` 설정은 그대로 유지된다.

업그레이드 후 첫 `pier` 실행 때는 `~/.tmux.conf`의 pier 블록을 최신 내용으로
갱신하고 tmux를 리로드한다(`tmux.conf: pier block updated`). 이때 블록이 가리키던
바이너리 경로는 그대로 유지되므로, 다른 위치의 pier를 한 번 실행했다고 해서
사용 중인 설정이 그쪽으로 바뀌지 않는다.

## 업데이트

```sh
brew update && brew upgrade zkzofn/tap/pier
```

또는 pier 안에서 `pier upgrade` — 하루 한 번 확인을 기다리지 않고 지금 바로 tap에
물어본 뒤 Homebrew 업그레이드까지 돌려준다. 소스 설치는 체크아웃에서
`git pull && make setup`.

사실 외울 필요는 없다. `pier`로 시작할 때 하루 한 번 tap을 확인해 알려주고, 새
버전이 있는 동안에는 모든 사이드바 하단에 `↑ <버전> · pier upgrade` 줄이 뜬다.
둘 다 `PIER_NO_UPDATE_CHECK=1`로 끌 수 있다.

업그레이드 후 설정이 뒤처지는 일도 없다 — pier의 tmux 훅이 도는 순간(= attach나
세션 전환 때마다) `~/.tmux.conf`의 pier 블록을 최신 내용으로 갱신하고 tmux를
리로드한다. 이때 블록에 적힌 바이너리 경로는 그대로 유지한다.

버전별로 무엇이 바뀌었는지는 [CHANGELOG.md](CHANGELOG.md)에 정리돼 있다.

<details>
<summary>수동 설정 — <code>pier setup</code>이 하는 일</summary>

### 1. tmux 설정

`~/.tmux.conf`에 추가:

```tmux
set -g mouse on

# 세션이 끝나면 클라이언트를 tmux 밖으로 내보내지 않고 최근 세션으로 이동
# (`exit`이 prefix+x와 같은 결과가 된다)
set -g detach-on-destroy off

set -g status-interval 5
set -g status-right-length 140
set -g status-right '#(~/.local/bin/pier status "#{pane_current_path}" "#{session_name}" "#{pane_id}" "#{client_width}") %H:%M '

# attach·세션 전환 시 사이드바 자동 생성 (이미 있으면 no-op)
set-hook -g client-attached 'run-shell "~/.local/bin/pier ensure"'
set-hook -g client-session-changed 'run-shell "~/.local/bin/pier ensure"'

# 사이드바만 남은 세션은 pane이 죽는 즉시 정리
set-hook -g pane-exited 'run-shell "~/.local/bin/pier reap"'

# prefix + g : 사이드바 토글
bind-key g run-shell "~/.local/bin/pier toggle"

# prefix + x : 현재 세션 종료 후 다음 세션으로 (사이드바 순서)
# (기본 kill-pane 바인딩을 덮어씀 — pane 닫기는 exit/Ctrl-D로)
bind-key x run-shell "~/.local/bin/pier done"

# prefix + N : 새 세션 picker 열기
bind-key N display-popup -E -w 46 -h 22 -T " New session " "~/.local/bin/pier new"
```

적용: `tmux source-file ~/.tmux.conf`

### 2. Claude Code hooks (상태 아이콘용)

`~/.claude/settings.json`의 `hooks`에 추가 (`pier`가 `PATH`에 있으면 그대로, 아니면 절대 경로):

```json
{
  "hooks": {
    "UserPromptSubmit": [{ "hooks": [{ "type": "command", "command": "pier hook user-prompt-submit", "timeout": 5 }] }],
    "PreToolUse": [{ "matcher": "*", "hooks": [{ "type": "command", "command": "pier hook pre-tool-use", "timeout": 5 }] }],
    "Stop": [{ "hooks": [{ "type": "command", "command": "pier hook stop", "timeout": 5 }] }],
    "PermissionRequest": [{ "matcher": "*", "hooks": [{ "type": "command", "command": "pier hook permission-request", "timeout": 5 }] }],
    "SessionStart": [{ "hooks": [{ "type": "command", "command": "pier hook session-start", "timeout": 5 }] }],
    "SessionEnd": [{ "hooks": [{ "type": "command", "command": "pier hook session-end", "timeout": 5 }] }]
  }
}
```

### 3. Antigravity (`agy`) hooks (선택 사항, `agy` 상태 아이콘용)

`~/.gemini/config/hooks.json`의 `"pier"` 키에 추가:

```json
{
  "pier": {
    "SessionStart": [{ "type": "command", "command": "pier hook session-start", "timeout": 5 }],
    "PreInvocation": [{ "type": "command", "command": "pier hook user-prompt-submit", "timeout": 5 }],
    "PreToolUse": [{ "matcher": "*", "hooks": [{ "type": "command", "command": "pier hook pre-tool-use", "timeout": 5 }] }],
    "PostToolUse": [{ "matcher": "*", "hooks": [{ "type": "command", "command": "pier hook pre-tool-use", "timeout": 5 }] }],
    "Stop": [{ "type": "command", "command": "pier hook stop", "timeout": 5 }]
  }
}
```

### 4. 상태바 잔여 사용량 캐시 (선택 사항)

- `~/.claude/settings.json`: `"statusLine": { "type": "command", "command": "pier statusline claude" }` (기존 statusLine 스크립트를 유지하려면 `--wrap '<기존 명령>'` 추가)
- `~/.gemini/antigravity-cli/settings.json`: `"statusLine": { "type": "command", "command": "pier statusline agy", "stack_with_default": true }`

hooks나 `statusLine` 없이도 사이드바·점프·경로/브랜치 status bar는 전부 동작한다. 상태 아이콘(`·`로 고정), 프롬프트 라벨, 사용량 표시, 크래시 복구만 빠진다.

</details>

## 사용법

`pier`(또는 `pier alias`로 따로 등록한 별칭)를 입력하면:

- **tmux 밖** — picker가 뜬다. 위쪽엔 실행 중인 세션(`Enter`로 그 세션에 attach — picker가 만들지 않고 합류하는 유일한 경우), 아래엔 새 세션을 시작할 디렉토리. 무엇을 고르든 pier가 attach까지 한다. 마지막 세션이 끝나면 `[exited]` 흔적 없이 원래 프롬프트로 돌아온다.
- **tmux 안** — 현재 pane에서 설정된 기본 에이전트(`pier agent`)가 실행된다.
- **`pier -r`, `pier --continue` …** — `-`로 시작하는 인자는 전부 기본 에이전트로 전달된다(pier 자신의 `-v` / `-h`만 예외). 모르는 단어는 usage를 띄우므로 오타로 세션이 시작되는 일은 없다.

`pier`로 시작할 때 하루 한 번 Homebrew tap에 새 버전이 있는지 확인해 업그레이드를 제안한다 — `PIER_NO_UPDATE_CHECK=1`로 끌 수 있고, 소스 설치는 프롬프트 대신 한 줄 안내만 나온다.

| 하고 싶은 것 | 방법 |
|---|---|
| 다른 세션으로 점프 | 사이드바 항목 **클릭** |
| 새 세션 생성 | `+ new session` 클릭(사이드바 포커스에서 `n`, 아무 데서나 `prefix+N`, tmux 밖에서는 그냥 `pier`). 타이핑으로 필터 — 실행 중인 세션과 디렉토리(`~/dev/*` + 과거 Claude Code·Antigravity 사용 경로) 양쪽에 걸린다. `Enter` 생성, `^A`로 설치된 에이전트 순환 전환, `^S`는 에이전트 대신 터미널 세션(일반 셸)으로 생성, `^T`는 텔레그램 채널 부착(Claude Code 전용), `Tab`으로 제안된 이름 수정. 없는 경로를 입력하면 `mkdir & create`로 디렉토리까지 만들어 진행. 실행 중인 세션은 맨 위에, 현재 세션이 `▌`로 가장 위에 오고 커서도 거기서 시작 — 실행 중 세션 행의 `Enter`는 그 디렉토리에 에이전트 세션 하나 더(`-2`, `-3`, … 접미사), `^S`는 그 디렉토리에 터미널. tmux 안에서 picker는 세션을 전환하지 않는다(전환은 사이드바 클릭). tmux 밖에서는 실행 중 세션 행의 `Enter`가 attach. 모든 키는 `▸` 커서 행에 작용하며, popup이 떠 있는 동안 그대로 누른다(prefix 불필요) |
| 죽은 세션 복구 | picker에서 크래시·셧다운으로 끊긴 Claude Code 대화가 있는 디렉토리엔 `↻ resume`이 표시된다: `→` 후 `Enter`면 이어서, 그냥 `Enter`면 빈 세션(복구 기회는 보존). 맨 위 `↻ restore all` 행(`↑` 한 번)은 사상자 전부를 각자의 세션으로 되살린다 |
| 키보드로 점프·스크롤 | 사이드바 pane으로 포커스 이동(`prefix+←`) 후 `j`/`k`(긴 목록 자동 스크롤) + `Enter`; 마우스 휠로도 스크롤 가능 |
| 기본 에이전트 변경 | `pier agent <이름>` (`claude`, `agy`, `codex`, `grok` 또는 커스텀 이름); 인자 없이 `pier agent`면 현재 설정과 사용 가능 목록을 출력 |
| 커스텀 에이전트 추가 | `~/.config/pier/config.json`에 `"agents": [{"name": "mycli", "cmd": "mycli --fast"}]` 추가 (내장 에이전트 `cmd` 오버라이드도 가능) |
| alias 변경·추가 | `pier alias <이름>`, 인자 없이 `pier alias`면 현재 설정을 보여준다 |
| pier 업데이트 | `pier upgrade` (또는 `brew upgrade zkzofn/tap/pier`) |
| 단축키 치트시트 | 사이드바 하단 `? help` 클릭, 또는 사이드바 포커스에서 `?` — 아무 키나 누르면 닫힘 |
| 사이드바 열기/닫기 | `prefix + g`, 또는 `pier toggle` |
| 현재 세션 끝내기 | `prefix + x` (또는 `pier done`) — 세션을 종료하고 사이드바 순서상 다음 세션으로 점프. 다른 세션이 없으면 그냥 종료. 확인 절차 없이 즉시 kill. tmux 기본 kill-pane 바인딩을 덮어씀. 세션의 마지막 pane에서 그냥 `exit`해도 같다 — 세션이 즉시 닫히고 클라이언트는 가장 최근 세션으로 옮겨간다 |
| 강제 새로고침 | 사이드바에서 `r` |
| 사이드바 종료 | 사이드바에서 `q` |

키 입력은 tmux가 포커스된 pane으로 보내므로, `j`/`k`는 사이드바에 포커스가 있을 때만 동작한다. 일상 사용은 클릭과 마우스 휠 스크롤이 기본이다.

## 동작 원리

```
┌─ tmux 세션 (세션마다 사이드바 1개) ─────────────┐
│ ┌────────┐  ┌──────────────────────────────┐  │
│ │  pier  │  │  AI CLI (claude/agy/…)       │  │
│ │  run   │  │                              │  │
│ └───┬────┘  └──────────────┬───────────────┘  │
└─────┼──────────────────────┼──────────────────┘
      │ 2s 폴링:              │ hooks / statusLine
      │ tmux list-panes -a   ▼
      │◀─ fsnotify ── ~/.local/state/pier/panes/<pane>.json
```

- **진실의 원천은 tmux.** 2초마다 `list-panes -a`를 폴링해 AI CLI pane을 판별한다(`claude` 또는 버전 문자열 `2.1.206` 형태, `agy`, `codex`, `grok` 및 `config.json`의 커스텀 `agents`). 상태 파일은 장식일 뿐이라 hook이 죽어도 목록은 항상 정확하다.
- **상태는 hooks가 기록.** `pier hook <event>`는 Claude Code(`~/.claude/settings.json`) 또는 Antigravity(`~/.gemini/config/hooks.json`)의 자식 프로세스로 실행되어 `$TMUX_PANE`을 상속받으므로 pane과 정확히 매핑된다. TUI는 fsnotify로 즉시 반영한다.
- **항목 라벨 = 현재 프롬프트.** 각 인스턴스 행에는 그 pane에서 마지막으로 제출한 프롬프트가 표시된다. `/clear`는 새 Claude Code 세션을 시작하므로(`SessionStart` hook) 다음 프롬프트 전까지 라벨이 공백이 된다. 아직 기록된 프롬프트가 없으면(설치 직후, resume) `~/.claude/projects/*/<id>.jsonl`의 `ai-title` 레코드(또는 첫 프롬프트), Antigravity의 `~/.gemini/antigravity-cli/cache/conversation_metadata.json`, tmux 세션명 순으로 폴백한다.
- **상태바 사용량 캐시.** `pier statusline claude`와 `pier statusline agy`는 각 CLI의 `statusLine` JSON을 `~/.local/state/pier/usage/<agent>.json`에 로컬 캐시한다(`--wrap`으로 기존 출력 유지). `codex`는 `~/.codex/sessions/**/rollout-*.jsonl`의 최근 rate limits를 읽고, `grok`은 표시를 생략한다. `pier status`는 현재 세션 활성 에이전트의 잔여 사용량과 경로·브랜치를 함께 출력한다.
- **크래시·셧다운 복구 (Claude Code).** `UserPromptSubmit`마다 생존 마커(`~/.claude/live-sessions/<session-id>.json`, 세션의 cwd·pid·tmux 세션명 포함)를 남기고, `SessionEnd`가 이를 종료 로그로 회수한다. picker는 다음 두 경우에 그 디렉토리를 사상자 보유로 표시한다:
  - *크래시* (SIGKILL, 커널 패닉, 전원 차단): hook이 돌지 못했으므로 마커가 죽은 pid를 담은 채 남아 있다.
  - *정상 OS 셧다운*: hook이 돌아 마커는 사라진다 — 대신 사이드바가 1분마다 touch하는 하트비트 파일이 이전 부팅에서 동결된 시각 ±90초 안에 끝난 세션을 셧다운 사상자로 판정한다. 사용자가 의도한 종료(`/clear`, logout, 프롬프트에서 exit)는 제외된다.

  복구는 언제나 명시적 선택이다 — `↻ resume` 버튼 또는 `↻ restore all`이며, 자동으로 이어붙이지 않는다. 실제로 resume했을 때만 기록이 소비되고, 빈 세션으로 시작하면 남는다. 기록은 7일이 지나거나 대화 트랜스크립트가 사라지면 만료된다. 한계: 셧다운 감지는 셧다운 당시 사이드바가 떠 있었어야 하고(하트비트의 주인), 셧다운 직전 ~90초 안에 직접 닫은 세션은 오탐으로 표시될 수 있다 — 무시해도 되는 제안일 뿐이다.

## 메모리 사용량 (실측)

iMac Pro(Intel Xeon W-2140B, 8코어/16스레드), macOS 15.7.9, Go 1.26.5, tmux 세션
8개가 떠 있는 상태에서 측정. `ps`와 활성 상태 보기가 보여주는 RSS 기준이고, 사이드바는
tmux 구조상 세션마다 1개씩 뜬다.

| 항목 | 측정값 |
|---|---|
| 사이드바 1개 | 14 ~ 16 MB |
| 사이드바 6개 합계 | 91.6 MB |
| `pier hook` / `pier status` | 상주 없음 (이벤트당 수 ms 단발 실행) |
| 바이너리 크기 | 10.6 MB |

시작 몇 초 안에 이 수준에 도달해 그대로 머문다 — 위 6개는 1시간 켜 둔 상태에서 각각
14.6 ~ 15.6 MB였다. 1.3.1까지는 처음 몇 분에 걸쳐 40 ~ 90 MB까지 올라갔고 머신의 코어
수에 비례했다. [changelog](CHANGELOG.md)의 1.4.0 항목 참고.

## 트러블슈팅

- **사이드바가 죽어 있음** → `prefix + g` 두 번 (제거 후 재생성)
- **discover가 뭘 보는지 확인** → `go run ./cmd/dbg`
- **마우스 텍스트 선택이 안 됨** → `mouse on`의 트레이드오프. iTerm2는 `Option+드래그`로 네이티브 선택
- **alias가 다른 걸 실행함** → `pier alias <다른이름>`으로 변경 가능. 다른 파일에서 source되는 동명 함수는 로드 순서에 따라 alias를 덮어쓸 수 있다
- **상태 아이콘이 전부 `·`** → hooks 미설정(`pier setup` 실행)이거나, 떠 있던 세션이 아직 새 프롬프트를 받지 않은 상태. 다음 프롬프트부터 기록된다
- **상태바에 에이전트 사용량이 안 뜸** → `pier setup` 실행 후 이미 떠 있던 `claude` / `agy` 세션을 재시작한다(`statusLine` 설정은 시작 시에만 읽음). `codex`는 세션 rollout 로그에 rate limit이 기록된 이후 표시된다

## 라이선스

MIT
