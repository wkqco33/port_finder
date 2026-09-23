# AGENTS.md — 프로젝트 개발 가이드

이 문서는 **poff(port_finder)** 저장소에서 작업하는 에이전트(인간/AI 모두)가 지켜야 할
구조적 규약과 **TDD 워크플로**, **CLI 가이드라인(clig.dev)** 준수 지침을 정의합니다.
코드를 수정하거나 추가하기 전에 반드시 읽고 따르세요.

## 프로젝트 개요

특정 포트를 점유한 프로세스를 찾아 안전하게 종료하고, AI(Ollama)로 포트 사용 현황을 분석하는 크로스플랫폼(Windows/Linux/macOS) CLI 도구.

- 언어: Go 1.25+
- CLI: `github.com/wkqco33/wcli`
- OS 연동: `github.com/shirou/gopsutil/v4` (net/process)
- LLM 클라이언트: `github.com/wkqco33/LLM_client_go`
- 출력/스타일: `github.com/fatih/color`

### 디렉터리 구조

```
main.go                진입점 (cmd.Execute() 호출만)
cmd/                   CLI 레이어 — wcli 명령 정의 + 입출력/흐름 오케스트레이션
  root.go              루트 커맨드 (포트 검색/종료, --ai, 플래그 및 스트림)
  config_cmd.go        config 서브커맨드 (show/init/set)
  check_cmd.go         check 서브커맨드 (로컬 포트 상태 / 원격 TCP 도달성)
pkg/
  port/                도메인 로직 — 포트 조회/프로세스 종료 (gopsutil 격리) + Checker(로컬/원격 확인)
  ai/                  AI 로직 — Ollama/LLM 포트 분석 및 프롬프트 빌더
  config/              설정 관리 — XDG 규약/환경변수/레거시 파일 로드/저장
.github/workflows/      CI 게이트 (ci.yml) + 릴리스 (release.yml)
Taskfile.yml           빌드/테스트 태스크 래퍼
```

## 아키텍처 규칙 (반드시 준수)

### 1. 의존성 방향

`main.go → cmd → pkg/port, pkg/ai, pkg/config`.
- **하위 계층(pkg/*)이 상위 계층(cmd)을 절대 참조하지 않는다.**
- `pkg/` 내부 패키지 간에도 순환 의존성을 만들지 않는다.

### 2. OS 및 외부 SDK 격리

- gopsutil 같은 OS 라이브러리 import는 `pkg/port` **외부 어디에도** 두지 마세요.
- LLM 클라이언트 SDK import는 `pkg/ai` **외부 어디에도** 두지 마세요.
- `cmd`는 오직 `pkg/`가 노출한 인터페이스와 타입만 사용합니다.

### 3. 의존성 주입 — TDD 전제

모든 패키지는 단위 테스트에서 외부 부수효과 없이 결정적으로 실행될 수 있도록 의존성 주입(DI)을 지원해야 합니다:

- `pkg/port`: **`Finder` 구조체 + `Option` 함수 옵션**
  - `port.NewFinder(port.WithConnectionSource(fake), port.WithProcessSource(fake))`
  - `Process`, `ConnectionSource`, `ProcessSource` 인터페이스를 통해서만 OS에 접근.
  - `port.NewChecker(port.WithFinder(f), port.WithDialer(fake))`: 원격 확인은 `Dialer` 인터페이스로만 네트워크에 접근(단위 테스트에서 실제 연결 금지).
- `pkg/ai`: **`Analyzer` 구조체 + `Option` 함수 옵션**
  - `ai.NewAnalyzer(ai.WithChatClient(fake), ai.WithModel(...), ai.WithBaseURL(...))`
  - `ChatClient` 인터페이스를 통해 실제 네트워크 통신 없이 가상 응답 테스트.
- `pkg/config`: **순수 함수 + 경로 주입**
  - `ResolvePath(homeDirFunc)`: 가상 홈 디렉터리 함수를 주입하여 XDG 및 레거시 경로 검증.
  - 파일 IO는 `t.TempDir()`로 격리하여 테스트.
- `cmd`: **`App` 및 `cfgApp` 구조체에 스트림, ops 인터페이스, TTY 함수 주입**
  - 필드: `Out`, `ErrW io.Writer`, `In io.Reader`, `Ops portOps`, `AI aiOps`, `IsTTY func() bool`
  - **코드 안에서 `os.Stdout`/`os.Stderr`/`os.Stdin`을 직접 쓰지 말 것.** 항상 `a.Out`/`a.ErrW`/`a.In`을 사용.
  - `fmt.Println`/`fmt.Printf`는 **절대 사용 금지** → `fmt.Fprintln(a.Out, ...)`/`fmt.Fprintf(a.Out, ...)` 사용.

### 4. CLI 가이드라인 준수 (clig.dev)

- **출력 스트림 엄격 분리**:
  - 결과 데이터는 `a.Out`(stdout)으로만 출력.
  - 진행 상태(`🔍...`, `스캔 중...`), 경고, 에러는 `a.ErrW`(stderr)로 출력.
  - `--json` 모드나 `-q/--quiet` 모드에서는 진행 메시지를 출력하지 않음.
  - `--json` 출력 시 stdout에는 오직 유효한 JSON만 출력되어야 하며, 검색 결과가 없을 때는 `[]` 빈 배열을 출력하여 파이프라인(`jq` 등)이 깨지지 않게 함.
- **대화형/비대화형 방어**:
  - 파괴적 작업(프로세스 kill) 확인 전에 반드시 `isTTY`와 `NoInput` 여부를 검사.
  - 비대화형 환경(파이프, CI)에서는 `--force(-f)` 또는 `--yes(-y)` 플래그가 없으면 즉시 종료 코드 `3(ExitCodePromptRequired)`으로 실패 처리.
- **표준 플래그 지원**:
  - `-h/--help`, `-v/--version`, `-f/--force`, `-y/--yes`, `-n/--dry-run`, `-q/--quiet`, `--no-input`, `--no-color`
- **표준 종료 코드 체계**:
  - `ExitCodeSuccess = 0`: 성공
  - `ExitCodeError = 1`: 일반 실행/종료 실패
  - `ExitCodeUsage = 2`: CLI 인자/플래그 파싱 실패
  - `ExitCodePromptRequired = 3`: 비대화형 환경에서 대화형 입력 필요

## 테스트 규칙 (TDD)

### 테스트 계층 구분 — 빌드 태그로 나뉜다

| 파일 | 태그 | 성격 |
| ------ | ------ | ------ |
| `pkg/port/port_unit_test.go` | `//go:build !integration` | **hermetic 단위 테스트** (페이크 주입). 기본 실행. |
| `pkg/port/check_unit_test.go` | `//go:build !integration` | **hermetic 단위 테스트** (페이크 소켓 소스/Dialer). 기본 실행. |
| `pkg/port/port_test.go` | `//go:build integration` | **통합 테스트** (실제 소켓/OS 프로세스). 별도 실행. |
| `pkg/port/check_integration_test.go` | `//go:build integration` | **통합 테스트** (실제 리스너/연결). 별도 실행. |
| `pkg/ai/ai_test.go` | (없음) | **hermetic 단위 테스트** (페이크 ChatClient). |
| `pkg/ai/prompt_test.go` | (없음) | **순수 단위 테스트** (프롬프트 빌더). |
| `pkg/ai/ai_integration_test.go` | `//go:build integration` | **통합 테스트** (로컬 Ollama 연결, 미실행 시 Skip). |
| `pkg/config/config_test.go` | (없음) | **hermetic 단위 테스트** (TempDir 파일 IO). |
| `cmd/root_test.go` | (없음) | **hermetic 단위 테스트** (페이크 ops/AI + 버퍼 스트림). |
| `cmd/config_cmd_test.go` | (없음) | **hermetic 단위 테스트** (페이크 경로 + 버퍼 스트림). |
| `cmd/check_cmd_test.go` | (없음) | **hermetic 단위 테스트** (페이크 check ops + 버퍼 스트림). |

- **기본 `go test ./...`는 반드시 빠르고(목표 < 1s), OS 비의존, 결정적이어야 합니다.**
  - 실제 네트워크를 열거나 OS 프로세스를 띄우는 테스트를 기본 스위트에 넣지 마세요 → `integration` 태그로 분리.
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
- 의존성 페이크 주입은 **함수 옵션** 또는 구조체 필드를 통해 수행.

### 실행 명령 (Taskfile)

```bash
task test             # hermetic 단위 테스트 (빠름, < 1s)
task test:integration # 실제 OS 통합 테스트
task test:all         # 둘 다 실행
task test:coverage    # 커버리지 측정
```

## CI 게이트 (.github/workflows/ci.yml)

- PR 및 main/master 푸시 시: `gofmt` 포맷 검증 → `go vet` 정적 분석 → 빌드 → `-race` 단위 테스트(커버리지 요약) → 통합 테스트 (`go test -tags integration ./...`).
- 릴리스는 별도 `release.yml` — 버전 태그(`v*`) 푸시 시 트리거되어 ppm 배포 규약(`{bin_name}_{os}_{arch}.tar.gz|.zip`)에 맞는 아카이브와 `.sha256` 체크섬을 생성한다.

## 코드 스타일

- Go 표준 `gofmt`/`go vet` 통과가 기본.
- 주석은 한국어로 작성 (기존 코드 관례 따름).
- 에러는 `fmt.Errorf("...: %w", err)`로 **래핑**(bare `fmt.Errorf` 사용 금지).
- 모든 공개 패키지에는 패키지 레벨 주석(`// Package ...`)을 작성.
- 빌드 태그 주석은 파일 최상단에 목적을 설명.

## 변경 시 반드시 확인

```bash
go build ./...        # 컴파일
go vet ./...          # 정적 분석
task test             # hermetic 단위 테스트
task test:integration # OS 의존 통합 테스트 (실제 환경 확인)
```
