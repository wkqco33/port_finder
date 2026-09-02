// ai_test.go는 Analyzer의 결정적 단위 테스트입니다.
// ChatClient 인터페이스를 페이크로 주입해 네트워크 호출 없이 로직을 검증합니다.
package ai

import (
	"context"
	"errors"
	"strings"
	"testing"

	llm "github.com/wkqco33/LLM_client_go"
)

// fakeChatClient는 ChatClient의 최소 페이크입니다.
type fakeChatClient struct {
	completeFunc func(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error)

	lastReq   llm.ChatRequest
	callCount int
}

func (f *fakeChatClient) Complete(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
	f.callCount++
	f.lastReq = req
	if f.completeFunc != nil {
		return f.completeFunc(ctx, req)
	}
	return &llm.ChatResponse{
		Choices: []llm.Choice{{Message: llm.Message{Content: "분석 결과"}}},
	}, nil
}

func newTestAnalyzer(client *fakeChatClient) *Analyzer {
	return NewAnalyzer(WithChatClient(client), WithModel("test-model"))
}

// ─── Analyze ──────────────────────────────────────────────────────────────────

func TestAnalyze_SendsModelAndMessages(t *testing.T) {
	client := &fakeChatClient{}
	analyzer := newTestAnalyzer(client)

	services := []Service{{Port: 3000, PID: 1234, Name: "node"}}
	_, err := analyzer.Analyze(context.Background(), services)
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}

	if client.lastReq.Model != "test-model" {
		t.Errorf("모델명 = %q, want %q", client.lastReq.Model, "test-model")
	}
	if len(client.lastReq.Messages) == 0 {
		t.Fatal("메시지가 비어 있습니다")
	}

	// system + user 메시지 구성 확인
	if client.lastReq.Messages[0].Role != llm.RoleSystem {
		t.Errorf("첫 메시지 role = %q, want %q", client.lastReq.Messages[0].Role, llm.RoleSystem)
	}
	userMsg := client.lastReq.Messages[len(client.lastReq.Messages)-1]
	if userMsg.Role != llm.RoleUser {
		t.Errorf("마지막 메시지 role = %q, want %q", userMsg.Role, llm.RoleUser)
	}
	if !strings.Contains(userMsg.Content, "3000") || !strings.Contains(userMsg.Content, "node") {
		t.Errorf("사용자 메시지에 서비스 정보가 누락되었습니다:\n%s", userMsg.Content)
	}
}

func TestAnalyze_ReturnsResponseContent(t *testing.T) {
	client := &fakeChatClient{
		completeFunc: func(_ context.Context, _ llm.ChatRequest) (*llm.ChatResponse, error) {
			return &llm.ChatResponse{
				Choices: []llm.Choice{{Message: llm.Message{Content: "포트 분석 완료"}}},
			}, nil
		},
	}
	analyzer := newTestAnalyzer(client)

	got, err := analyzer.Analyze(context.Background(), []Service{{Port: 80, PID: 1, Name: "nginx"}})
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	if got != "포트 분석 완료" {
		t.Errorf("Analyze() = %q, want %q", got, "포트 분석 완료")
	}
}

func TestAnalyze_EmptyChoicesReturnsError(t *testing.T) {
	client := &fakeChatClient{
		completeFunc: func(_ context.Context, _ llm.ChatRequest) (*llm.ChatResponse, error) {
			return &llm.ChatResponse{Choices: nil}, nil
		},
	}
	analyzer := newTestAnalyzer(client)

	_, err := analyzer.Analyze(context.Background(), nil)
	if err == nil {
		t.Fatal("빈 Choices에 대해 에러가 반환되어야 합니다")
	}
}

func TestAnalyze_WrapsClientError(t *testing.T) {
	sentinel := errors.New("연결 거부됨")
	client := &fakeChatClient{
		completeFunc: func(_ context.Context, _ llm.ChatRequest) (*llm.ChatResponse, error) {
			return nil, sentinel
		},
	}
	analyzer := newTestAnalyzer(client)

	_, err := analyzer.Analyze(context.Background(), nil)
	if err == nil {
		t.Fatal("클라이언트 에러가 전파되어야 합니다")
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("에러가 래핑되지 않았습니다: %v", err)
	}
}

func TestAnalyze_ContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	client := &fakeChatClient{}
	analyzer := newTestAnalyzer(client)

	if _, err := analyzer.Analyze(ctx, nil); !errors.Is(err, context.Canceled) {
		t.Errorf("취소된 컨텍스트 에러 = %v, want context.Canceled", err)
	}
}

// ─── Option ───────────────────────────────────────────────────────────────────

func TestWithBaseURL(t *testing.T) {
	analyzer := NewAnalyzer(WithBaseURL("http://127.0.0.1:9999/v1"))
	if analyzer.baseURL != "http://127.0.0.1:9999/v1" {
		t.Errorf("baseURL = %q", analyzer.baseURL)
	}
}

func TestDefaultModel(t *testing.T) {
	analyzer := NewAnalyzer()
	if analyzer.model != DefaultModel {
		t.Errorf("기본 모델 = %q, want %q", analyzer.model, DefaultModel)
	}
}
