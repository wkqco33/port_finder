package cmd

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"port_finder/pkg/ai"
	portpkg "port_finder/pkg/port"
)

// ─── 페이크 ops ───────────────────────────────────────────────────────────────

type fakeOps struct {
	findFunc  func(uint16) ([]*portpkg.ProcessInfo, error)
	rangeFunc func(uint16, uint16) ([]*portpkg.ProcessInfo, error)
	listFunc  func() ([]*portpkg.ProcessInfo, error)
	killFunc  func(int32) error
	graceFunc func(int32, time.Duration) error

	killed []int32
}

func (f *fakeOps) FindByPort(p uint16) ([]*portpkg.ProcessInfo, error) { return f.findFunc(p) }
func (f *fakeOps) FindByPortRange(s, e uint16) ([]*portpkg.ProcessInfo, error) {
	return f.rangeFunc(s, e)
}
func (f *fakeOps) ListAll() ([]*portpkg.ProcessInfo, error) { return f.listFunc() }
func (f *fakeOps) KillProcessByPID(pid int32) error {
	f.killed = append(f.killed, pid)
	if f.killFunc != nil {
		return f.killFunc(pid)
	}
	return nil
}
func (f *fakeOps) KillProcessGracefully(pid int32, t time.Duration) error {
	f.killed = append(f.killed, pid)
	if f.graceFunc != nil {
		return f.graceFunc(pid, t)
	}
	return nil
}

func newAppForTest(ops portOps, in string, out *bytes.Buffer) *App {
	return &App{
		Out:  out,
		ErrW: out,
		In:   strings.NewReader(in),
		Ops:  ops,
	}
}

// ─── ParsePortArg ─────────────────────────────────────────────────────────────

func TestParsePortArg(t *testing.T) {
	tests := []struct {
		input     string
		wantStart uint16
		wantEnd   uint16
		wantErr   bool
	}{
		{"8080", 8080, 8080, false},
		{" 8080 ", 8080, 8080, false},
		{"3000-4000", 3000, 4000, false},
		{" 3000 - 4000 ", 3000, 4000, false},
		{"0", 0, 0, true},
		{"70000", 0, 0, true},
		{"3000-2000", 0, 0, true}, // lo > hi
		{"abc", 0, 0, true},
		{"3000-abc", 0, 0, true},
		{"abc-4000", 0, 0, true},
		{"3000-4000-5000", 0, 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			start, end, err := ParsePortArg(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParsePortArg(%q) error = %v, wantErr = %v", tt.input, err, tt.wantErr)
			}
			if !tt.wantErr {
				if start != tt.wantStart || end != tt.wantEnd {
					t.Errorf("ParsePortArg(%q) = (%d, %d), want (%d, %d)", tt.input, start, end, tt.wantStart, tt.wantEnd)
				}
			}
		})
	}
}

// ─── runSinglePort ───────────────────────────────────────────────────────────

func TestRunSinglePort_NotFound(t *testing.T) {
	ops := &fakeOps{findFunc: func(uint16) ([]*portpkg.ProcessInfo, error) { return nil, nil }}
	var out bytes.Buffer
	app := newAppForTest(ops, "n\n", &out)
	app.PortStr = "8080"

	if err := app.Run(nil); err != nil {
		t.Fatalf("Run 오류: %v", err)
	}
	if !strings.Contains(out.String(), "찾을 수 없습니다") {
		t.Errorf("미발견 메시지가 없습니다: %q", out.String())
	}
	if len(ops.killed) != 0 {
		t.Errorf("미발견 시 종료가 호출되면 안 됩니다: %v", ops.killed)
	}
}

func TestRunSinglePort_ConfirmDecline_NoKill(t *testing.T) {
	ops := &fakeOps{findFunc: singleResult}
	var out bytes.Buffer
	app := newAppForTest(ops, "n\n", &out)
	app.PortStr = "8080"

	if err := app.Run(nil); err != nil {
		t.Fatalf("Run 오류: %v", err)
	}
	if len(ops.killed) != 0 {
		t.Errorf("사용자가 거부했는데 종료가 호출됨: %v", ops.killed)
	}
	if !strings.Contains(out.String(), "취소") {
		t.Errorf("취소 메시지가 없습니다: %q", out.String())
	}
}

func TestRunSingle_ConfirmAccepted_Kills(t *testing.T) {
	ops := &fakeOps{findFunc: singleResult}
	var out bytes.Buffer
	app := newAppForTest(ops, "y\n", &out)
	app.PortStr = "8080"

	if err := app.Run(nil); err != nil {
		t.Fatalf("Run 오류: %v", err)
	}
	if len(ops.killed) != 1 || ops.killed[0] != 1234 {
		t.Errorf("종료 호출 = %v, 기대 [1234]", ops.killed)
	}
}

func TestRunSingle_ForceKill_NoPrompt(t *testing.T) {
	ops := &fakeOps{findFunc: singleResult}
	var out bytes.Buffer
	app := newAppForTest(ops, "", &out)
	app.PortStr = "8080"
	app.ForceKill = true

	if err := app.Run(nil); err != nil {
		t.Fatalf("Run 오류: %v", err)
	}
	if len(ops.killed) != 1 {
		t.Errorf("강제 종료 시 KillProcessByPID가 호출되어야 합니다: %v", ops.killed)
	}
}

func TestRunJSON_OutputsValidJSON(t *testing.T) {
	ops := &fakeOps{findFunc: singleResult}
	var out bytes.Buffer
	app := newAppForTest(ops, "", &out)
	app.PortStr = "8080"
	app.JSON = true

	if err := app.Run(nil); err != nil {
		t.Fatalf("Run 오류: %v", err)
	}
	json := out.String()
	if !strings.Contains(json, `"port": 8080`) || !strings.Contains(json, `"pid": 1234`) {
		t.Errorf("JSON 출력에 필드 누락: %q", json)
	}
}

// ─── runPortRange ─────────────────────────────────────────────────────────────

func TestRunPortRange_Empty(t *testing.T) {
	ops := &fakeOps{rangeFunc: func(uint16, uint16) ([]*portpkg.ProcessInfo, error) { return nil, nil }}
	var out bytes.Buffer
	app := newAppForTest(ops, "", &out)
	app.PortStr = "3000-4000"

	if err := app.Run(nil); err != nil {
		t.Fatalf("Run 오류: %v", err)
	}
	if !strings.Contains(out.String(), "찾을 수 없습니다") {
		t.Errorf("미발견 메시지가 없습니다: %q", out.String())
	}
}

func TestRunPortRange_ForceKill_All(t *testing.T) {
	ops := &fakeOps{rangeFunc: rangeResults}
	var out bytes.Buffer
	app := newAppOp(ops, "", &out)
	app.PortStr = "3000-4000"
	app.ForceKill = true

	if err := app.Run(nil); err != nil {
		t.Fatalf("Run 오류: %v", err)
	}
	if len(ops.killed) != 2 {
		t.Errorf("두 프로세스 모두 종료되어야 합니다: %v", ops.killed)
	}
}

// ─── runList ──────────────────────────────────────────────────────────────────

func TestRunList_Empty(t *testing.T) {
	ops := &fakeOps{listFunc: func() ([]*portpkg.ProcessInfo, error) { return nil, nil }}
	var out bytes.Buffer
	app := newAppForTest(ops, "", &out)
	app.ListMode = true

	if err := app.Run(nil); err != nil {
		t.Fatalf("Run 오류: %v", err)
	}
	if !strings.Contains(out.String(), "사용 중인 포트가 없습니다") {
		t.Errorf("빈 목록 메시지가 없습니다: %q", out.String())
	}
}

func TestRunList_Table(t *testing.T) {
	ops := &fakeOps{listFunc: func() ([]*portpkg.ProcessInfo, error) {
		return []*portpkg.ProcessInfo{{PID: 1, Name: "node", Port: 3000}}, nil
	}}
	var out bytes.Buffer
	app := newAppForTest(ops, "", &out)
	app.ListMode = true

	if err := app.Run(nil); err != nil {
		t.Fatalf("Run 오류: %v", err)
	}
	s := out.String()
	if !strings.Contains(s, "PORT") || !strings.Contains(s, "node") {
		t.Errorf("테이블 출력에 헤더/값 누락: %q", s)
	}
}

// ─── 헬퍼 ─────────────────────────────────────────────────────────────────────

func singleResult(uint16) ([]*portpkg.ProcessInfo, error) {
	return []*portpkg.ProcessInfo{{PID: 1234, Name: "node", Port: 8080}}, nil
}

func rangeResults(uint16, uint16) ([]*portpkg.ProcessInfo, error) {
	return []*portpkg.ProcessInfo{
		{PID: 100, Name: "a", Port: 3000},
		{PID: 200, Name: "b", Port: 3500},
	}, nil
}

func newAppOp(ops *fakeOps, in string, out *bytes.Buffer) *App {
	return newAppForTest(ops, in, out)
}

// ─── AI 모드 ──────────────────────────────────────────────────────────────────

type fakeAI struct {
	analyzeFunc func(services []ai.Service) (string, error)

	lastServices []ai.Service
	callCount    int
}

func (f *fakeAI) Analyze(ctx context.Context, services []ai.Service) (string, error) {
	f.callCount++
	f.lastServices = services
	if f.analyzeFunc != nil {
		return f.analyzeFunc(services)
	}
	return "AI 분석 결과입니다.", nil
}

func TestRunAI_ListAnalysis(t *testing.T) {
	ops := &fakeOps{listFunc: func() ([]*portpkg.ProcessInfo, error) {
		return []*portpkg.ProcessInfo{{PID: 1, Name: "node", Port: 3000}}, nil
	}}
	aiOps := &fakeAI{}
	var out bytes.Buffer
	app := newAppForTest(ops, "", &out)
	app.AIMode = true
	app.AI = aiOps

	if err := app.Run(nil); err != nil {
		t.Fatalf("Run 오류: %v", err)
	}

	s := out.String()
	if !strings.Contains(s, "node") {
		t.Errorf("포트 테이블이 출력되어야 합니다: %q", s)
	}
	if !strings.Contains(s, "AI 분석 결과입니다.") {
		t.Errorf("AI 분석 결과가 출력되어야 합니다: %q", s)
	}
	if aiOps.callCount != 1 {
		t.Errorf("Analyze 호출 횟수 = %d, want 1", aiOps.callCount)
	}
	if len(aiOps.lastServices) != 1 || aiOps.lastServices[0].Name != "node" {
		t.Errorf("변환된 서비스가 올바르지 않습니다: %+v", aiOps.lastServices)
	}
}

func TestRunAI_NoKillPrompt(t *testing.T) {
	ops := &fakeOps{listFunc: func() ([]*portpkg.ProcessInfo, error) {
		return []*portpkg.ProcessInfo{{PID: 1, Name: "node", Port: 3000}}, nil
	}}
	aiOps := &fakeAI{}
	var out bytes.Buffer
	app := newAppForTest(ops, "", &out)
	app.AIMode = true
	app.AI = aiOps

	if err := app.Run(nil); err != nil {
		t.Fatalf("Run 오류: %v", err)
	}
	if strings.Contains(out.String(), "종료하시겠습니까") {
		t.Error("AI 모드에서는 종료 확인 프롬프트가 출력되지 않아야 합니다")
	}
	if len(ops.killed) != 0 {
		t.Errorf("AI 모드에서는 프로세스를 종료하지 않아야 합니다: %v", ops.killed)
	}
}

func TestRunAI_LLMError(t *testing.T) {
	ops := &fakeOps{listFunc: func() ([]*portpkg.ProcessInfo, error) {
		return []*portpkg.ProcessInfo{{PID: 1, Name: "node", Port: 3000}}, nil
	}}
	aiOps := &fakeAI{analyzeFunc: func([]ai.Service) (string, error) {
		return "", errors.New("연결 거부됨")
	}}
	var out bytes.Buffer
	app := newAppForTest(ops, "", &out)
	app.AIMode = true
	app.AI = aiOps

	if err := app.Run(nil); err == nil {
		t.Fatal("LLM 실패 시 에러가 반환되어야 합니다")
	}
}

func TestRunAI_PortAndAIMutuallyExclusive(t *testing.T) {
	ops := &fakeOps{}
	aiOps := &fakeAI{}
	var out bytes.Buffer
	app := newAppForTest(ops, "", &out)
	app.AIMode = true
	app.PortStr = "8080"
	app.AI = aiOps

	err := app.Run(nil)
	if err == nil {
		t.Fatal("--port와 --ai 동시 사용 시 에러가 반환되어야 합니다")
	}
	if aiOps.callCount != 0 {
		t.Errorf("에러 시 Analyze가 호출되지 않아야 합니다: %d", aiOps.callCount)
	}
}

func TestRunAI_AIOnlyMode(t *testing.T) {
	// --ai 단독 사용 시에도 목록 기반 분석이 수행되어야 합니다.
	ops := &fakeOps{listFunc: func() ([]*portpkg.ProcessInfo, error) {
		return []*portpkg.ProcessInfo{{PID: 1, Name: "node", Port: 3000}}, nil
	}}
	aiOps := &fakeAI{}
	var out bytes.Buffer
	app := newAppForTest(ops, "", &out)
	app.AIMode = true
	app.AI = aiOps

	if err := app.Run(nil); err != nil {
		t.Fatalf("Run 오류: %v", err)
	}
	if aiOps.callCount != 1 {
		t.Errorf("--ai 단독 실행 시 Analyze가 호출되어야 합니다: %d", aiOps.callCount)
	}
}

func TestRunAI_EmptyList(t *testing.T) {
	ops := &fakeOps{listFunc: func() ([]*portpkg.ProcessInfo, error) { return nil, nil }}
	aiOps := &fakeAI{}
	var out bytes.Buffer
	app := newAppForTest(ops, "", &out)
	app.AIMode = true
	app.AI = aiOps

	if err := app.Run(nil); err != nil {
		t.Fatalf("Run 오류: %v", err)
	}
	if aiOps.callCount != 0 {
		t.Errorf("빈 목록에서는 Analyze가 호출되지 않아야 합니다: %d", aiOps.callCount)
	}
}

// ─── AI 비활성 모드 ───────────────────────────────────────────────────────────

func TestRunWithoutAIFlag_ListModeSkipsAI(t *testing.T) {
	// -l만 사용 시 --ai 플래그가 없으면 AI 분석이 실행되지 않아야 합니다.
	ops := &fakeOps{listFunc: func() ([]*portpkg.ProcessInfo, error) {
		return []*portpkg.ProcessInfo{{PID: 1, Name: "node", Port: 3000}}, nil
	}}
	aiOps := &fakeAI{}
	var out bytes.Buffer
	app := newAppForTest(ops, "", &out)
	app.ListMode = true
	app.AI = aiOps // newApp()이 AI를 항상 구성하므로 AIMode 플래그로만 제어됩니다.

	if err := app.Run(nil); err != nil {
		t.Fatalf("Run 오류: %v", err)
	}
	if aiOps.callCount != 0 {
		t.Errorf("--ai 플래그 없이는 Analyze가 호출되지 않아야 합니다: %d", aiOps.callCount)
	}
	if !strings.Contains(out.String(), "node") {
		t.Error("일반 목록 모드가 실행되어야 합니다")
	}
}

func TestRunWithoutAIFlag_SinglePortSkipsAI(t *testing.T) {
	// -p 8080은 --ai 플래그 없이 AI와 무관하게 동작해야 합니다.
	ops := &fakeOps{findFunc: singleResult}
	aiOps := &fakeAI{}
	var out bytes.Buffer
	app := newAppForTest(ops, "y", &out)
	app.PortStr = "8080"
	app.AI = aiOps

	if err := app.Run(nil); err != nil {
		t.Fatalf("Run 오류: %v", err)
	}
	if aiOps.callCount != 0 {
		t.Errorf("--ai 플래그 없이는 Analyze가 호출되지 않아야 합니다: %d", aiOps.callCount)
	}
}
