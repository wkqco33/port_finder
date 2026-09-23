# Changelog

모든 주요 변경 사항은 이 파일에 기록됩니다.

형식은 [Keep a Changelog](https://keepachangelog.com/ko/1.1.0/) 기반이며,
버저닝은 [Semantic Versioning](https://semver.org/lang/ko/)을 따릅니다.

## [Unreleased]

## [0.5.0] - 2026-09-23

### 추가

- `check` 서브커맨드: 포트 상태 확인 기능
  - 로컬 확인(`poff check -p 8080`): 포트 바인딩 여부와 프로토콜/주소/상태(LISTEN, NONE 등)/점유 프로세스를 출력. TCP는 `LISTEN`, UDP는 바인딩 여부로 수신 대기를 판정
  - 로컬 범위 확인(`poff check -p 8000-8010`): 범위 내 사용 중인 포트를 표로 출력(미사용 포트는 제외)
  - 원격 확인(`poff check -H 192.168.0.10 -p 8080`): ICMP(ping)와 달리 **포트 단위 TCP 도달성**을 확인하고 `open`/`closed`/`filtered`/`unreachable`/`error`로 판정
  - 플래그: `-p/--port`, `-H/--host`, `-t/--timeout`(기본 3s), `-j/--json`, `-q/--quiet`, `--no-color`
  - 원격 범위 확인은 최대 16개까지 병렬 시도하며 결과는 포트 오름차순으로 출력
  - `--json`은 stdout에만 유효한 JSON 배열을 출력(바인딩/결과가 없으면 `[]`), 진행/요약은 stderr
  - 확인 결과에 따른 종료 코드: 수신 대기 또는 연결 성공(`open`)이면 `0`, 그 외는 `1`
- `pkg/port`에 `Finder.BindingsByPortRange`(소켓 바인딩 상세)와 `Checker`(`CheckLocal`/`CheckLocalRange`/`CheckRemote`, `Dialer` 주입) 추가
- OS 비의존 단위 테스트(페이크 `ConnectionSource`/`Dialer`) 및 실제 소켓 통합 테스트(`-tags integration`) 추가

### 수정

- 알 수 없는 플래그/서브커맨드 등 CLI 파싱 실패 시 종료 코드가 `1`이 아닌 `2(ExitCodeUsage)`로 반환되도록 수정

## [0.4.0] - 2026-09-13

### 추가

- CLI 표준 플래그 추가:
  - `-y, --yes`: 프로세스 즉시 종료 (`--force` 별칭)
  - `-n, --dry-run`: 실제 프로세스를 종료하지 않고 시뮬레이션
  - `-q, --quiet`: 진행 및 안내 메시지 억제
  - `--no-input`: 비대화형 모드 강제
  - `--no-color`: ANSI 컬러 출력 비활성화
- 표준 종료 코드(Exit Code) 체계 도입:
  - `0`: 성공
  - `1`: 실행/프로세스 종료 오류
  - `2`: 잘못된 사용법/인자 오류
  - `3`: 비대화형 환경에서 대화형 입력 필요
- 설정 파일 XDG Base Directory 규약 및 `POFF_CONFIG` 환경변수 지원:
  - 1순위: `POFF_CONFIG` 환경변수
  - 2순위: 기존 레거시 파일 (`~/.poff.json`)
  - 3순위 (기본값): `$XDG_CONFIG_HOME/poff/config.json` (또는 Windows `%APPDATA%\poff\config.json`)

### 수정

- `--json` 출력 시 stdout에 진행 안내 메시지(`🔍...`)가 출력되어 JSON 파서(`jq` 등)가 깨지는 문제 해결 (진행 메시지를 stderr로 분리 및 억제)
- `--json` 출력 시 검색 결과가 없을 때 일반 텍스트 대신 빈 JSON 배열 `[]` 출력 보장
- 비TTY 터미널 및 `--no-input` 환경에서 `--force`/`--yes` 플래그 없이 실행 시 블록되지 않고 종료 코드 3과 함께 안내 메시지 출력

## [0.3.0] - 2026-09-02

### 추가

- `config` 서브커맨드(`show`/`init`/`set`): 설정 파일(`config.json`)로 AI 모델/엔드포인트/타임아웃 영속 관리
- `--ai-base-url`, `--ai-timeout` 플래그: 설정 파일 값을 일회성으로 덮어쓰기
- `--ai` 플래그: LLM(Ollama, 기본 `qwen3:4b`)으로 현재 사용 중 포트를 분석하는 기능 — 서비스 용도 추정, 위험도, 정리 제안 출력 (분석 전용, 종료 없음)
- `--ai-model` 플래그: AI 분석에 사용할 Ollama 모델 지정

## [0.2.0] - 2026-08-23

### 추가

- MIT 라이선스 적용 및 커뮤니티 파일(보안 정책, 기여 가이드, 행동 강령) 추가
- GitHub Actions 액션 최신화 및 CI/release 파이프라인 재구성
- ppm 배포 규약에 맞는 바이너리 아카이브(`.tar.gz`/`.zip`) 및 SHA-256 체크섬 빌드

## [0.1.5] - 2026-06-27

### 추가

- 포트 범위 스캔 기능

### 변경

- 의존성 및 릴리스 워크플로 갱신

## [0.1.4] - 2026-06-27

### 변경

- 빌드 시스템 Taskfile 기반으로 재구성

## [0.1.3] - 2026-03-01

### 수정

- 빌드 워크플로 버그 수정

## [0.1.2] - 2026-03-01

### 변경

- 패키지 메타데이터 개선

## [0.1.1] - 2026-02-28

### 추가

- 릴리스 워크플로에 write 권한 부여

## [0.1.0] - 2026-02-28

### 추가

- 초기 릴리스: 포트 스캔 및 프로세스 종료 기능

[Unreleased]: https://github.com/wkqco33/port_finder/compare/v0.5.0...HEAD
[0.5.0]: https://github.com/wkqco33/port_finder/compare/v0.4.0...v0.5.0
[0.4.0]: https://github.com/wkqco33/port_finder/compare/v0.3.0...v0.4.0
[0.3.0]: https://github.com/wkqco33/port_finder/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/wkqco33/port_finder/compare/v0.1.5...v0.2.0
[0.1.5]: https://github.com/wkqco33/port_finder/releases/tag/v0.1.5
[0.1.4]: https://github.com/wkqco33/port_finder/releases/tag/v0.1.4
[0.1.3]: https://github.com/wkqco33/port_finder/releases/tag/v0.1.3
[0.1.2]: https://github.com/wkqco33/port_finder/releases/tag/v0.1.2
[0.1.1]: https://github.com/wkqco33/port_finder/releases/tag/v0.1.1
[0.1.0]: https://github.com/wkqco33/port_finder/releases/tag/v0.1.0
