# 기여 가이드 (Contributing Guide)

`poff`에 기여해 주셔서 감사합니다! 아래 지침을 따라 원활하게 협업할 수 있습니다.

## 시작하기

1. 저장소를 포크하고 작업 브랜치를 만듭니다.
2. 변경 사항은 작게, 논리 단위로 유지합니다.
3. 작업 전에 [Issues](https://github.com/wkqco33/port_finder/issues)에서 기존 논의를 확인하거나 새 Issue를 엽니다.

## 개발 환경

- **Go 1.26.1+** (go.mod의 `go` 지시문과 일치)
- 빌드/테스트는 [Taskfile.yml](Taskfile.yml)을 사용합니다.

```bash
# hermetic 단위 테스트 (빠름, OS 비의존)
task test

# 통합 테스트 (실제 OS 소켓/프로세스)
task test:integration

# 빌드
task build
```

## 브랜치 및 커밋

- 작업 브랜치 이름: `feature/설명` 또는 `fix/설명`
- 커밋 메시지는 **Conventional Commits** 형식을 권장합니다.
  - `feat:`, `fix:`, `docs:`, `refactor:`, `test:`, `chore:`, `ci:`, `perf:`
- 관련 Issue가 있으면 커밋 본문에 `Closes #123` 형식으로 연결합니다.

## PR 절차

1. 변경 사항을 커밋하고 push한 뒤 Pull Request를 엽니다.
2. PR을 열기 전에 다음이 **반드시 통과**해야 합니다.

   ```bash
   gofmt -l .        # 포맷 준수 (출력 없어야 함)
   go vet ./...       # 정적 분석
   go test ./...      # hermetic 단위 테스트
   ```

3. CI (`ci.yml`)가 통과하는지 확인합니다.
4. 변경 사항에는 **테스트를 함께 포함**합니다. (AGENTS.md TDD 규칙 참조)
5. 리뷰어의 피드백에 대응합니다.

## 코드 스타일

- Go 표준 `gofmt` / `go vet` 준수.
- 주석은 한국어로 작성 (기존 코드 관례).
- 아키텍처 규칙 및 TDD 워크플로는 [AGENTS.md](AGENTS.md)를 참고하세요.

## 이슈 등록 가이드

- **버그**: 재현 단계, 예상 동작, 실제 동작, OS/버전을 포함.
- **기능 제안**: 목적과 사용 시나리오를 포함.

## 행동 강령

모든 참여자는 [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md)를 준수해야 합니다.
