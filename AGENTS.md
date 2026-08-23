# AGENTS.md — 프로젝트 개발 가이드

이 문서는 **poff(port_finder)** 저장소에서 작업하는 에이전트(인간/AI 모두)가 지켜야 할
구조적 규약과 **TDD 워크플로**를 정의합니다. 코드를 고치기 전에 반드시 읽고 따르세요.

## 프로젝트 개요

특정 포트를 점유한 프로세스를 찾아 안전하게 종료하는 크로스플랫폼(Windows/Linux/macOS) CLI 도구.

- 언어: Go 1.25+
- CLI: `github.com/wkqco33/wcli`
- OS 연동: `github.com/shirou/gopsutil/v4` (net/process)
- 출력/스타일: `github.com/fatih/color`

### 디렉터리 구조

```
main.go            진입점 (cmd.Execute() 호출만)
cmd/               CLI 레이어 — wcli 명령 정의 + 입출력/흐름 오케스트레이션
pkg/port/          도메인 로직 — 포트 조회/프로세스 종료 (gopsutil 격리)
.github/workflows/  CI 테스트 게이트 (test.yml) + 릴리스 (release.yml)
Taskfile.yml       빌드/테스트 태스크 래퍼
```

## 아키텍처 규칙 (반드시 준수)

### 1. 의존성 방향

`main.go → cmd → pkg/port`. **하위 계층(pkg/port)이 상위 계층(cmd)을 절대 참조하지 않는다.**

### 2. OS 연동은 `pkg/port`에만 존재

gopsutil 같은 OS 라이브러리 import는 `pkg/port` **외부 어디에도** 두지 마세요.
`cmd`는 오직 `pkg/port`가 노출한 API만 사용합니다.

### 3. 의존성 주입 — 테스트를 위해 (TDD 전제)

- `pkg/port`: **`Finder` 구조체 + `Option` 함수 옵션**이 정석입니다.
  - `port.NewFinder()` → 실제 소스 사용
  - `port.NewFinder(port.WithConnectionSource(fake), port.WithProcessSource(fake))` → 테스트용 페이크 주입
  - `Process`, `ConnectionSource`, `ProcessSource` 인터페이스를 통해서만 OS에 접근하세요.
- `cmd`: **`App` 구조체**에 스트림과 ops를 주입합니다.
  - 필드: `Out`, `ErrW io.Writer`, `In io.Reader`, `Ops portOps`(인터페이스)
  - **함수 안에서 `os.Stdout`/`os.Stderr`/`os.Stdin`을 직접 쓰지 말 것.** 항상 `a.Out`/`a.ErrW`/`a.In`을 사용.
  - `fmt.Println`/`fmt.Printf`는 **절대 사용 금지** → `fmt.Fprintln(a.Out, ...)`/`fmt.Fprintf(a.Out, ...)` 사용.

### 4. 순수 함수는 부수효과와 분리

파싱(`ParsePortArg`), 변환, 정렬 같은 순수 로직은 입출력과 분리해 두어야 단위 테스트가 쉽다.

## 테스트 규칙 (TDD)

### 테스트 계층 구분 — 빌드 태그로 나뉜다

| 파일 | 태그 | 성격 |
| ------ | ------ | ------ |
| `pkg/port/port_unit_test.go` | `//go:build !integration` | **hermetic 단위 테스트** (페이크 주입). 기본 실행. |
| `pkg/port/port_test.go` | `//go:build integration` | **통합 테스트** (실제 소켓/OS 프로세스). 별도 실행. |
| `cmd/root_test.go` | (없음) | hermetic 단위 테스트 (페이크 ops + `bytes.Buffer`). |

- **기본 `go test ./...`는 반드시 빠르고(목표 < 1s), OS 비의존, 결정적이어야 합니다.**
  - 실제 네트워크를 열거나 OS 프로세스를 띄우는 테스트를 기본 스위트에 넣지 마세요 → `integration` 태그로.
  - `time.Sleep`으로 "바인딩 대기" 같은 비결정적 대기를 쓰지 마세요 → 페이크로 대체.
- **통합 테스트는 `go test -tags integration ./...`** 로 실행합니다.

### 프로세스: Red → Green → Refactor

1. 먼저 실패하는 테스트를 작성 (`Red`).
2. 최소한의 구현으로 통과 (`Green`).
3. 의존성/인터페이스를 개선하며 리팩터 (`Refactor`).
4. 테스트와 프로덕션 코드를 **같은 커밋**에 포함하라.

### 테스트 작성 규칙

- 테스트 함수명: `Test<기능>_<시나리오>` (예: `TestFindByPortRange_Sorted`).
- 순수 함수는 **테이블 드리븐**(`tests := []struct{...}` + `t.Run`)으로.
- 페이크는 테스트 파일 안에 정의하고, 작게(인터페이스 만족 최소) 유지.
- 의존성 페이크 주입은 **함수 옵션**을 통해 (`WithConnectionSource` 등).

### 실행 명령 (Taskfile)

```bash
task test             # hermetic 단위 테스트 (빠름)
task test:integration # 실제 OS 통합 테스트
task test:all         # 둘 다
task test:coverage    # 커버리지
```

## CI 게이트 (.github/workflows/test.yml)

- PR 및 main 푸시 시: `go vet` → `-race` 단위 테스트 → 커버리지 요약 → ubuntu 통합 테스트.
- 릴리스는 별도 `release.yml` (태그 `v*`).

## 코드 스타일

- Go 표준 `gofmt`/`go vet` 통과가 기본.
- 주석은 한국어로 작성 (기존 코드 관례 따름).
- 에러는 `fmt.Errorf("...: %w", err)`로 **래핑**(bare `fmt.Errorf` 사용 금지).
- 빌드 태그 주석은 파일 최상단에 목적을 설명.

## 변경 시 반드시 확인

```bash
go build ./...        # 컴파일
go vet ./...          # 정적 분석
task test             # hermetic 단위 테스트
task test:integration # OS 의존 통합 테스트 (실제 환경 확인)
```

- 새 OS 라이브러리 import를 추가했다면 반드시 `pkg/port`에 **인터페이스 페이크**를 동반하라.
- CLI 출력을 바꿨다면 `cmd` 테스트가 `bytes.Buffer`로 검증하는지 확인하라.
