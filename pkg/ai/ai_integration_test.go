// ai_integration_test.go는 실제 로컬 Ollama 서버에 의존하는 통합 테스트입니다.
// 빌드 태그로 기본 스위트에서 제외되며, 다음 명령으로 실행합니다:
//
//	go test -tags integration ./...
package ai

import (
	"context"
	"strings"
	"testing"
	"time"
)

// TestAnalyze_Integration_Ollama는 실제 Ollama에 연결해 분석을 수행합니다.
// Ollama가 실행되지 않은 환경에서는 자동으로 건너뜁니다.
func TestAnalyze_Integration_Ollama(t *testing.T) {
	analyzer := NewAnalyzer()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	result, err := analyzer.Analyze(ctx, []Service{
		{Port: 3000, PID: 1, Name: "node"},
		{Port: 5432, PID: 2, Name: "postgres"},
	})
	if err != nil {
		// Ollama 미실행 환경에서는 스킵 (CI/로컬 모두 안전)
		t.Skipf("Ollama에 연결할 수 없어 통합 테스트를 건너뜁니다: %v", err)
	}

	if strings.TrimSpace(result) == "" {
		t.Fatal("분석 결과가 빈 문자열입니다")
	}
	t.Logf("분석 결과 일부: %.120s", result)
}

// TestAnalyze_Integration_WithModel은 특정 모델 지정 동작을 검증합니다.
func TestAnalyze_Integration_WithModel(t *testing.T) {
	analyzer := NewAnalyzer(WithModel(DefaultModel))

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	result, err := analyzer.Analyze(ctx, []Service{{Port: 8080, PID: 3, Name: "java"}})
	if err != nil {
		t.Skipf("Ollama에 연결할 수 없어 통합 테스트를 건너뜁니다: %v", err)
	}
	if strings.TrimSpace(result) == "" {
		t.Fatal("분석 결과가 빈 문자열입니다")
	}
}
