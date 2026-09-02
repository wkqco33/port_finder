// ai.go는 LLM 분석기(Analyzer)를 제공합니다.
// LLM 클라이언트는 ChatClient 인터페이스로 추상화되어 테스트에서 페이크로 대체할 수 있으며,
// 기본 구현은 LLM_client_go의 Ollama 클라이언트(로컬, OpenAI 호환)입니다.
package ai

import (
	"context"
	"fmt"
	"strings"
	"time"

	llm "github.com/wkqco33/LLM_client_go"
	"github.com/wkqco33/LLM_client_go/ollama"
	"github.com/wkqco33/LLM_client_go/openai"
)

// DefaultModel은 AI 분석에 사용되는 기본 Ollama 모델입니다.
const DefaultModel = "qwen3:4b"

// DefaultBaseURL은 기본 Ollama 엔드포인트입니다.
const DefaultBaseURL = "http://localhost:11434/v1"

// ChatClient는 이 패키지가 필요로 하는 LLM 클라이언트의 최소 표면입니다.
// llm.Client 인터페이스의 부분 집합으로, 테스트에서 페이크로 대체할 수 있습니다.
type ChatClient interface {
	Complete(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error)
}

// openai.Client가 ChatClient를 만족하는지 컴파일 시점에 검증합니다.
var _ ChatClient = (*openai.Client)(nil)

const systemPrompt = "당신은 시스템 포트 사용 현황을 분석하는 전문가입니다. " +
	"주어진 포트/프로세스 목록을 분석하여 정확하고 간결하게 한국어로 답변하세요."

// Analyzer는 포트 서비스 목록을 LLM으로 분석합니다.
// 의존성(클라이언트, 모델)을 주입 가능하여 결정적 단위 테스트를 지원합니다.
type Analyzer struct {
	client  ChatClient
	model   string
	baseURL string
	timeout time.Duration
}

// Option은 Analyzer 구성을 변경하는 함수 옵션입니다.
type Option func(*Analyzer)

// WithChatClient는 LLM 클라이언트를 교체합니다. (테스트용)
func WithChatClient(c ChatClient) Option {
	return func(a *Analyzer) { a.client = c }
}

// WithModel은 분석에 사용할 모델명을 설정합니다.
func WithModel(model string) Option {
	return func(a *Analyzer) { a.model = model }
}

// WithBaseURL은 LLM 엔드포인트 URL을 설정합니다.
// 클라이언트를 직접 주입하지 않은 경우에만 적용됩니다.
func WithBaseURL(url string) Option {
	return func(a *Analyzer) { a.baseURL = url }
}

// WithTimeout은 LLM 요청 타임아웃을 설정합니다.
func WithTimeout(d time.Duration) Option {
	return func(a *Analyzer) { a.timeout = d }
}

// NewAnalyzer는 기본 설정(로컬 Ollama)의 Analyzer를 생성합니다.
// 옵션으로 테스트용 클라이언트 주입 등 구성을 변경할 수 있습니다.
func NewAnalyzer(opts ...Option) *Analyzer {
	a := &Analyzer{
		model:   DefaultModel,
		baseURL: DefaultBaseURL,
		timeout: 60 * time.Second,
	}
	for _, o := range opts {
		o(a)
	}
	// 클라이언트가 주입되지 않은 경우 기본 Ollama 클라이언트를 사용합니다.
	// 재시도(지수 백오프)는 LLM_client_go의 기본 정책이 자동 적용됩니다.
	if a.client == nil {
		a.client = ollama.New(ollama.Config{
			BaseURL: a.baseURL,
			Timeout: a.timeout,
		})
	}
	return a
}

// Analyze는 서비스 목록을 LLM에 보내 분석 결과 텍스트를 반환합니다.
func (a *Analyzer) Analyze(ctx context.Context, services []Service) (string, error) {
	// 취소된 컨텍스트는 요청 전에 즉시 실패합니다.
	if err := ctx.Err(); err != nil {
		return "", err
	}
	resp, err := a.client.Complete(ctx, llm.ChatRequest{
		Model: a.model,
		Messages: []llm.Message{
			{Role: llm.RoleSystem, Content: systemPrompt},
			{Role: llm.RoleUser, Content: BuildPrompt(services)},
		},
	})
	if err != nil {
		// 컨텍스트 취소/타임아웃은 그대로 전파합니다.
		if ctxErr := ctx.Err(); ctxErr != nil {
			return "", ctxErr
		}
		// 모델 미설치(404)는 별도 안내를 제공합니다.
		var apiErr *llm.APIError
		if llm.IsAPIError(err, &apiErr) && apiErr.StatusCode == 404 {
			return "", fmt.Errorf("모델 %q를 찾을 수 없습니다: ollama pull %s 를 실행하세요: %w", a.model, a.model, err)
		}
		return "", fmt.Errorf("LLM 분석 요청 실패 (Ollama가 실행 중인지 확인하세요: ollama serve): %w", err)
	}
	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("LLM 응답에 선택지가 없습니다 (모델 %q 응답이 비정상입니다)", a.model)
	}
	return strings.TrimSpace(resp.Choices[0].Message.Content), nil
}
