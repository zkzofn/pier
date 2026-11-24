# 2026-09-10 — 사이드바 메모리 증가 원인 조사

## 배경

README `## Memory usage (measured)` 표는 사이드바 하나를 7.1–9.7 MB (avg 8.1 MB)로
적고 있으나, 장호님 맥 실측이 크게 어긋났다.

- 생성 직후 RSS 14 MB
- 1~3분 뒤 20~41 MB
- 1~4일 켜 둔 사이드바 77~83 MB (vmmap physical footprint 57 MB)
- Linux 샌드박스(tmux 3.4, 소스 빌드) 생성 직후 16.5 MB

Threads 홍보 봇(@pier.cli)의 유일한 사실 출처가 이 표라, 표가 틀리면 봇 글도 틀린다.

측정 머신: iMac Pro, Intel Xeon W-2140B 8코어/16스레드, macOS 15.7.9, go1.26.5.
**README 표의 "macOS (Apple Silicon)"과 다른 머신이다** — 이 점이 아래 결론과 직접 얽힌다.

## 실측 1 — 시간 비례 증가가 아니다

장호님 상시 사이드바 7개의 스냅샷(RSS / vmmap physical footprint):

| uptime | RSS | footprint |
|---|---|---|
| 4분 | 62 MB | 42.0 MB |
| 15분 | 77 MB | 46.2 MB |
| 18분 | 74 MB | 41.3 MB |
| 1시간 11분 | 77 MB | 35.6 MB |
| 13시간 31분 | 87 MB | 54.5 MB |
| 1일 10시간 | 85 MB | 48.7 MB |
| 3일 6시간 | 80 MB | **43.9 MB** |

3일짜리(43.9 MB)가 4분짜리(42.0 MB)와 사실상 같다. **시간에 비례해 자라는 누수가 아니다.**
초반 몇 분 안에 도달한 수준에서 톱니 모양으로 진동할 뿐이다.

## 실측 2 — 시작 직후 값은 README 표와 일치한다

소스(1.3.1)를 빌드해 테스트 tmux 세션에서 사이드바를 띄운 직후 RSS = **7.7 MB**.
README 표의 7.1–9.7 MB는 조작이 아니라 **시작 직후 스냅샷**이 맞다.
같은 프로세스가 1분 17초 뒤 52.9 MB, 4분 34초 뒤 66.2 MB가 된다.

`GODEBUG=gctrace=1`:

```
gc 1 @0.156s   7->7->4 MB,  7 MB goal
gc 3 @4.018s  18->18->12 MB, 18 MB goal
gc 4 @8.154s  28->28->20 MB, 28 MB goal
gc 5 @24.157s 40->40->16 MB, 41 MB goal   ← live 16 MB, goal 41 MB
```

live heap이 4 → 16~20 MB로 커지고, Go의 기본 GOGC=100은 목표 힙을 live의 2배로 잡는다.
그래서 RSS는 40~55 MB대가 된다.

## 원인 — ansi 파서 풀의 4MB 버퍼 × 코어 수

heap profile (`inuse_space`) 상위:

```
Showing nodes accounting for 18439.56kB, 100% of 18439.56kB total
16389.50kB 88.88%  github.com/charmbracelet/x/ansi.(*Parser).SetDataSize
```

**inuse 18.4 MB 중 16.4 MB(88.9%)가 ansi 파서 버퍼 하나뿐이다.** 호출 경로:

```
ui.Model.View → lipgloss.Style.Render → cellbuf.Wrap → ansi.GetParser → (sync.Pool.New) SetDataSize
```

`github.com/charmbracelet/x/ansi@v0.10.1/parser_sync.go:13`:

```go
var parserPool = sync.Pool{
	New: func() any {
		p := NewParser()
		p.SetParamsSize(parser.MaxParamsSize)
		p.SetDataSize(1024 * 1024 * 4) // 4MB of data buffer
		return p
	},
}
```

풀에서 꺼내는 파서 하나가 **4 MB**다(일반 `NewParser()`는 64 KB). `sync.Pool`은 P별 캐시라
렌더가 여러 P에 흩어지면 파서 인스턴스가 그만큼 생긴다 — 상한이 GOMAXPROCS다.
16.4 MB ÷ 4 MB ≈ 4개.

즉 **누수가 아니라 머신 코어 수에 비례하는 상수 오버헤드**다. 몇 분이면 포화되고,
그 뒤로는 GC 타이밍과 P 분포에 따라 진동한다. 3일짜리가 4분짜리보다 크지 않은 이유가 이것이다.

## 검증 — GOMAXPROCS를 바꾸면 그대로 따라온다

같은 바이너리를 GOMAXPROCS만 바꿔 동시에 띄운 결과(2분 경과 시점):

| GOMAXPROCS | RSS |
|---|---|
| 16 (기본) | 66.2 MB |
| 4 | 30.1 MB |
| 2 | 17.7 MB |
| 1 | 13.7 MB |

코어 수에 비례한다. 인과 확정.

장호님이 보고한 Linux 샌드박스 "생성 직후 16.5 MB"도 같은 그림에 들어맞는다 —
생성 직후는 아직 풀이 안 찼고, 컨테이너는 대개 P가 적다. 다만 그 환경은 이번에
직접 재지 않았으므로 **추정**이다.

README 표의 8 MB가 나온 Apple Silicon 머신은 코어가 더 적고(M 시리즈 8~12), 무엇보다
**표가 시작 직후에 측정된 값**이라 파서 풀이 채워지기 전이다. 두 요인이 겹쳐 8 MB가 나왔다.

## 해결 — 업스트림이 이미 고쳤다

`x/ansi` v0.11.x의 같은 줄:

```go
p.SetDataSize(1024 * 4) // 4KB of data buffer
```

4 MB → 4 KB, 1000배 감소. pier는 v0.10.1을 indirect로 물고 있었다.

`x/ansi` 단독 업그레이드는 cellbuf와 API가 깨진다. `x/cellbuf@v0.0.15` + `x/ansi@v0.11.8`을
같이 올리면 **lipgloss v1.1.0 / bubbletea v1.3.10을 그대로 둔 채** 빌드된다.

- `go test ./...` 전부 통과
- 업그레이드 빌드 사이드바 RSS: 아래 "결과" 참조

## 결과

같은 머신·같은 16 P·같은 pier 코드, 의존성만 다른 두 빌드:

| 빌드 | gctrace live heap | RSS |
|---|---|---|
| ansi v0.10.1 (현재) | 16–20 MB, goal 41 MB | 74.2 MB (5분 37초) |
| ansi v0.11.8 + cellbuf v0.0.15 | **0 MB 일정** (GC 18회 내내 `3->3->0 MB, 4 MB goal`) | 14.2 MB (1분 20초) |

업그레이드 빌드는 힙이 아예 자라지 않는다. 파서 버퍼가 4 KB라 GC 통계에 잡히지도 않는 수준이다.
RSS 14 MB는 Go 런타임 + 바이너리 + 터미널 버퍼의 바닥값에 가깝다.

업그레이드 빌드의 heap profile (`inuse_space`) — 총 5.1 MB, **파서가 목록에서 사라졌다**:

```
Showing nodes accounting for 5124.84kB, 100% of 5124.84kB total
 3075.58kB 60.01%  runtime.mallocgc
  513.12kB 10.01%  sync.(*Pool).pinSlow
  512.08kB  9.99%  internal/sync.newIndirectNode
  512.05kB  9.99%  github.com/charmbracelet/bubbletea.Tick.func1
```

(`main.init.0.func1` 512 kB는 이번 조사용으로 임시로 넣은 pprof 서버라 실제 빌드엔 없다.)

## 9/7 벤치 결과와의 대조

`~/dev/pier-cli/bench/results/2026-09-07-macos-pier1.3.1.json` (같은 머신, 같은 1.3.1):

```
sidebar_rss: {t0: 13.4 MB, t10s: 15.2 MB, t30s: 16.7 MB}
binary: 10.7 MB
```

**30초까지만 잰다.** 파서 풀이 P를 다 채우기 전이라 16.7 MB에서 끝난다.
같은 빌드를 몇 분 더 두면 40~90 MB로 간다 — 이 벤치 스크립트의 관측 창(t0/60s/180s)이
포화 시점을 놓치는 구조였다. 표의 8 MB도, 벤치의 16.7 MB도, 실사용의 80 MB도
전부 같은 바이너리의 값이다. 조건을 안 적으면 어느 쪽도 거짓말이 된다.

덧붙여 README `Binary size | 5 MB` 도 틀렸다. 실제 10.6 MB(설치본 10.7 MB), Makefile에
`-s -w` 같은 스트립 플래그가 없다.

## README 표를 어떻게 고칠 것인가

지금 표가 부정직한 지점은 값 자체가 아니라 **조건이 빠진 것**이다. "measured"라고만 적혀 있어
독자는 정상 상태 값으로 읽는다. 최소한 다음이 명시돼야 한다.

1. 측정 시점 — 시작 직후인지, 포화 후인지
2. 측정 머신의 코어 수 — 값이 코어 수에 비례하므로
3. RSS인지 physical footprint인지

## 이력

- 2026-09-10 최초 작성. 원인 특정 및 업스트림 수정 확인.
