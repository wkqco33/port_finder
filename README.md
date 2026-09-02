# poff (Port Finder)

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![CI](https://github.com/wkqco33/port_finder/actions/workflows/ci.yml/badge.svg)](https://github.com/wkqco33/port_finder/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/wkqco33/port_finder?include_prereleases)](https://github.com/wkqco33/port_finder/releases)
[![Go Version](https://img.shields.io/github/go-mod/go-version/wkqco33/port_finder)](https://go.dev/dl/)
[![GitHub Repo stars](https://img.shields.io/github/stars/wkqco33/port_finder?style=social)](https://github.com/wkqco33/port_finder)

특정 포트를 사용 중인 프로세스를 찾아내고 안전하게 종료할 수 있도록 도와주는 크로스플랫폼 CLI 유틸리티입니다.

> ⚠️ **주의**: 종료 작업은 기본적으로 사용자 확인 후 진행되며, `-f` 옵션 사용 시 확인 없이 즉시 종료됩니다. 관리 권한이 필요한 프로세스는 권한에 따라 동작이 제한될 수 있습니다.

## 주요 기능

- **포트 스캔**: 지정한 포트를 점유하고 있는 프로세스(PID)와 프로세스명을 빠르게 찾아냅니다.
- **포트 범위 스캔**: `3000-4000`처럼 범위를 지정하여 사용 중인 포트를 한 번에 조회합니다.
- **전체 목록 조회**: 현재 열려 있는 모든 포트와 프로세스를 테이블로 출력합니다.
- **프로세스 종료**: 검색된 프로세스를 사용자 확인 후 안전하게 강제 종료(Kill)할 수 있습니다.
- **Graceful 종료**: SIGTERM을 먼저 보내고 5초 후 응답 없으면 SIGKILL로 에스컬레이션합니다.
- **AI 포트 분석**: `--ai` 옵션으로 현재 사용 중인 포트 전체를 LLM(Ollama)에게 분석시켜 서비스 용도 추정, 위험도, 정리 제안을 받습니다.
- **JSON 출력**: 스크립트 자동화 파이프라인을 위한 JSON 형식 출력을 지원합니다.
- **크로스플랫폼**: Linux, macOS, Windows 모두 지원합니다.

## 시스템 요구사항

- Go 1.26.1 이상

## 설치

### ppm (추천)

```bash
ppm install wkqco33/port_finder
```

### 직접 빌드

```bash
git clone https://github.com/wkqco33/port_finder.git
cd port_finder

# 빌드 (port_finder 또는 port_finder.exe 생성)
task build

# ~/.local/bin 에 설치
task install

# 설치 제거
task uninstall

# 빌드 결과물 삭제
task clean
```

## 사용법

```bash
poff [flags]

Flags:
  -p, --port string    검색할 포트 번호 또는 범위 (예: 8080, 3000-4000)
  -f, --force          확인 없이 즉시 프로세스 종료
  -l, --list           현재 사용 중인 모든 포트 목록 출력
  -j, --json           JSON 형식으로 출력
  -g, --graceful       SIGTERM 후 5초 대기, 이후 SIGKILL (Graceful 종료)
  -a, --ai             LLM(Ollama)으로 현재 사용 중 포트를 분석 (목록 분석 전용)
      --ai-model       AI 분석에 사용할 Ollama 모델 (기본: 설정값 또는 qwen3:4b)
      --ai-base-url    AI 분석에 사용할 LLM 엔드포인트 (기본: 설정값 또는 http://localhost:11434/v1)
      --ai-timeout     AI 분석 요청 타임아웃 (기본: 설정값 또는 1m, 예: 90s)
  -v, --version        버전 출력
  -h, --help           도움말 출력

Config Commands:
  poff config show     현재 유효 설정 표시 (출처 포함)
  poff config init     기본값 설정 파일 생성 (~/.poff.json)
  poff config set KEY VALUE  설정 값 변경 (예: poff config set ai.model llama3.2)
```

### 실행 예시

#### **단일 포트 검색 및 종료**

```text
$ poff -p 8080
🔍 포트 8080 사용 중인 프로세스를 검색 중입니다...

✨ 발견! 프로세스 상세 정보
   • PID        : 1234
   • NAME       : node
   • PORT       : 8080

🔥 해당 프로세스들을 즉시 종료하시겠습니까? (y/N): y

✅ PID 1234 프로세스가 안전하게 종료되었습니다.
```

#### **범위 스캔**

```bash
# 3000~9000 범위에서 사용 중인 포트 조회
poff -p 3000-9000

# 범위 내 모든 프로세스 확인 없이 즉시 종료
poff -p 3000-9000 -f
```

#### **전체 포트 목록**

```bash
# 테이블 형식으로 출력
poff -l

# JSON 형식으로 출력 (jq 등과 조합 가능)
poff -l -j | jq '.[] | select(.name == "node")'
```

#### **Graceful 종료**

```bash
# SIGTERM → 5초 대기 → SIGKILL 순으로 안전 종료
poff -p 8080 -g
```

#### **확인 없이 즉시 종료 (스크립트 자동화)**

```bash
poff -p 8080 -f
```

#### **AI 포트 분석 (Ollama)**

```bash
# 로컬 Ollama(기본 모델: qwen3:4b)로 현재 포트 사용 현황 분석
poff --ai

# 모델 지정
poff --ai --ai-model llama3.2
```

사전 준비:

```bash
ollama serve              # Ollama 서버 실행 (http://localhost:11434)
ollama pull qwen3:4b      # 분석에 사용할 모델 설치
```

AI 모드는 **분석 전용**입니다 — 서비스 용도 추정, 위험도, 정리 제안을 출력하며
프로세스를 종료하지 않습니다. 종료는 기존처럼 `poff -p <PORT> [-f]`로 수행하세요.

#### **AI 설정 관리 (config)**

```bash
# 기본값 설정 파일(~/.poff.json) 생성
poff config init

# 현재 유효 설정 확인 (출처: 플래그/설정 파일/기본값)
poff config show

# 설정 변경
poff config set ai.model llama3.2
poff config set ai.base_url http://192.168.1.5:11434/v1
poff config set ai.timeout 90s
```

설정 파일(`~/.poff.json`) 예시:

```json
{
  "ai": {
    "model": "qwen3:4b",
    "base_url": "http://localhost:11434/v1",
    "timeout": "1m0s"
  }
}
```

설정 우선순위는 **CLI 플래그 > 설정 파일 > 기본값**이며, 일회성 변경은
`poff --ai --ai-model llama3.2`처럼 플래그로 덮어쓸 수 있습니다.

#### **JSON 출력**

```bash
$ poff -p 8080 -j
[
  {
    "port": 8080,
    "pid": 1234,
    "name": "node"
  }
]
```

## 디렉터리 구조

- `main.go` : 진입점
- `cmd/` : CLI 커맨드 정의 (wcli)
- `pkg/port/` : 포트 조회 및 프로세스 종료 로직
- `pkg/ai/` : LLM(Ollama) 포트 분석 로직 (LLM_client_go)
- `pkg/config/` : 설정 파일(~/.poff.json) 로드/초기화/변경
- `Taskfile.yml` : 빌드/설치/제거 명령어 래퍼 (Task)
- `ppm.json` : ppm 패키지 메타데이터

## 기여 및 커뮤니티

이 프로젝트에 기여하고 싶다면 다음 문서를 참고해 주세요.

- [기여 가이드](CONTRIBUTING.md)
- [보안 정책](SECURITY.md) — 취약점 보고
- [행동 강령](CODE_OF_CONDUCT.md)
- [변경 이력](CHANGELOG.md)
- [라이선스](LICENSE)

기능 요청이나 버그 리포트는 [Issues](https://github.com/wkqco33/port_finder/issues)에, 질문이나 토론은 [Discussions](https://github.com/wkqco33/port_finder/discussions)에 남겨 주세요.

## 라이선스

[MIT](LICENSE) 라이선스 하에 배포됩니다. © 2026 poff contributors
