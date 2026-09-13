# Changelog

모든 주요 변경 사항은 이 파일에 기록됩니다.

형식은 [Keep a Changelog](https://keepachangelog.com/ko/1.1.0/) 기반이며,
버저닝은 [Semantic Versioning](https://semver.org/lang/ko/)을 따릅니다.

## [Unreleased]

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

[Unreleased]: https://github.com/wkqco33/port_finder/compare/v0.4.0...HEAD
[0.4.0]: https://github.com/wkqco33/port_finder/compare/v0.3.0...v0.4.0
[0.3.0]: https://github.com/wkqco33/port_finder/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/wkqco33/port_finder/compare/v0.1.5...v0.2.0
[0.1.5]: https://github.com/wkqco33/port_finder/releases/tag/v0.1.5
[0.1.4]: https://github.com/wkqco33/port_finder/releases/tag/v0.1.4
[0.1.3]: https://github.com/wkqco33/port_finder/releases/tag/v0.1.3
[0.1.2]: https://github.com/wkqco33/port_finder/releases/tag/v0.1.2
[0.1.1]: https://github.com/wkqco33/port_finder/releases/tag/v0.1.1
[0.1.0]: https://github.com/wkqco33/port_finder/releases/tag/v0.1.0
