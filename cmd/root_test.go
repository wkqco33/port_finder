package cmd

import (
	"bytes"
	"strings"
	"testing"
	"time"

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
