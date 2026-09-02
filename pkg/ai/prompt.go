// Package ai는 LLM을 활용해 포트 사용 현황을 분석하는 기능을 제공합니다.
// LLM 클라이언트(LLM_client_go)에만 의존하며 OS 연동은 포함하지 않습니다.
package ai

import (
	"fmt"
	"strings"
)

// Service는 분석 대상이 되는 포트-프로세스 정보입니다.
// pkg/port의 ProcessInfo와 독립적인 타입으로, cmd 계층에서 매핑됩니다.
type Service struct {
	Port uint16
	PID  int32
	Name string
}

// BuildPrompt는 서비스 목록을 LLM 분석용 프롬프트(사용자 메시지)로 변환하는 순수 함수입니다.
// 입출력 부수효과가 없어 단위 테스트가 쉽습니다.
func BuildPrompt(services []Service) string {
	var b strings.Builder

	b.WriteString("아래는 현재 시스템에서 사용 중인 포트와 프로세스 목록입니다.\n")
	b.WriteString("각 항목을 분석해서 다음 형식으로 한국어로 정리해 주세요:\n")
	b.WriteString("1. 전체 요약 (몇 개의 포트, 어떤 종류의 서비스가 실행 중인지)\n")
	b.WriteString("2. 포트별 분석 (추정 서비스 용도, 개발/운영/시스템 분류)\n")
	b.WriteString("3. 주의가 필요한 포트 (보안상 위험하거나 리소스를 많이 쓰는 항목)\n")
	b.WriteString("4. 정리 제안 (종료해도 무방해 보이는 항목과 이유)\n\n")
	b.WriteString("| PORT | PID | NAME |\n")
	b.WriteString("|------|-----|------|\n")

	for _, s := range services {
		fmt.Fprintf(&b, "| %d | %d | %s |\n", s.Port, s.PID, s.Name)
	}

	return b.String()
}
