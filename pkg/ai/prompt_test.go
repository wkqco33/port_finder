// prompt_test.go는 BuildPrompt 순수 함수의 결정적 단위 테스트입니다.
// 네트워크나 OS에 의존하지 않고 프롬프트 구성 로직만 검증합니다.
package ai

import (
	"strconv"
	"strings"
	"testing"
)

func TestBuildPrompt(t *testing.T) {
	tests := []struct {
		name         string
		services     []Service
		wantContains []string
	}{
		{
			name: "단일 서비스",
			services: []Service{
				{Port: 3000, PID: 1234, Name: "node"},
			},
			wantContains: []string{"3000", "1234", "node"},
		},
		{
			name: "여러 서비스",
			services: []Service{
				{Port: 3000, PID: 1234, Name: "node"},
				{Port: 5432, PID: 5678, Name: "postgres"},
				{Port: 8080, PID: 9999, Name: "java"},
			},
			wantContains: []string{"3000", "node", "5432", "postgres", "8080", "java"},
		},
		{
			name:         "빈 목록도 프롬프트는 생성된다",
			services:     []Service{},
			wantContains: []string{}, // 구조만 유효하면 됨
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildPrompt(tt.services)

			if strings.TrimSpace(got) == "" {
				t.Fatal("BuildPrompt가 빈 문자열을 반환했습니다")
			}

			for _, want := range tt.wantContains {
				if !strings.Contains(got, want) {
					t.Errorf("프롬프트에 %q가 포함되어야 합니다.\n프롬프트:\n%s", want, got)
				}
			}
		})
	}
}

func TestBuildPrompt_ContainsAllServiceFields(t *testing.T) {
	services := []Service{
		{Port: 5432, PID: 42, Name: "postgres"},
		{Port: 6379, PID: 99, Name: "redis-server"},
	}

	got := BuildPrompt(services)

	for _, s := range services {
		for _, field := range []string{
			strconv.Itoa(int(s.Port)),
			strconv.Itoa(int(s.PID)),
			s.Name,
		} {
			if !strings.Contains(got, field) {
				t.Errorf("프롬프트에 %q 누락", field)
			}
		}
	}
}
