// check_cmd_test.go는 `poff check` 서브커맨드(로컬 바인딩 / 원격 도달성)의 hermetic 단위 테스트입니다.
// 페이크 ops와 버퍼 스트림을 주입해 실제 네트워크/OS 없이 결정적으로 검증합니다.
package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	portpkg "port_finder/pkg/port"
)

// ─── 페이크 check ops ────────────────────────────────────────────────────────

// fakeCheckOps는 checkOps를 대체하는 페이크입니다.
// CheckRemote는 check 커맨드가 병렬로 호출하므로 내부 상태를 뮤텍스로 보호합니다.
type fakeCheckOps struct {
	mu sync.Mutex

	localFunc  func(start, end uint16) ([]*portpkg.LocalPortResult, error)
	remoteFunc func(host string, port uint16) *portpkg.ReachResult

	localCalls  [][2]uint16
	remoteCalls []string
}

func (f *fakeCheckOps) CheckLocalRange(start, end uint16) ([]*portpkg.LocalPortResult, error) {
	f.mu.Lock()
	f.localCalls = append(f.localCalls, [2]uint16{start, end})
	f.mu.Unlock()

	if f.localFunc != nil {
		return f.localFunc(start, end)
	}
	return nil, nil
}

func (f *fakeCheckOps) CheckRemote(ctx context.Context, host string, port uint16) *portpkg.ReachResult {
	f.mu.Lock()
	f.remoteCalls = append(f.remoteCalls, fmt.Sprintf("%s:%d", host, port))
	f.mu.Unlock()

	if f.remoteFunc != nil {
		return f.remoteFunc(host, port)
	}
	return &portpkg.ReachResult{Host: host, Port: port, Status: portpkg.ReachOpen}
}

func (f *fakeCheckOps) dialedPorts() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.remoteCalls...)
}

func (f *fakeCheckOps) localRanges() [][2]uint16 {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([][2]uint16(nil), f.localCalls...)
}

// ─── 헬퍼 ─────────────────────────────────────────────────────────────────────

// newCheckAppForTest는 버퍼 스트림과 페이크 ops로 checkApp을 구성합니다.
func newCheckAppForTest(ops checkOps, portStr, host string) (*checkApp, *bytes.Buffer, *bytes.Buffer) {
	out, errW := &bytes.Buffer{}, &bytes.Buffer{}
	app := &checkApp{
		Out:     out,
		ErrW:    errW,
		Ops:     ops,
		PortStr: portStr,
		Host:    host,
		Timeout: time.Second,
	}
	return app, out, errW
}

// localListening은 수신 대기 중인 단일 포트 결과를 만듭니다.
func localListening(port uint16) []*portpkg.LocalPortResult {
	return []*portpkg.LocalPortResult{{
		Port:      port,
		Listening: true,
		Bindings: []portpkg.Binding{{
			Port: port, IP: "0.0.0.0", Proto: "tcp", State: "LISTEN", PID: 1234, Name: "node",
		}},
	}}
}

// localEstablished는 LISTEN 아닌 바인딩만 있는 결과를 만듭니다.
func localEstablished(port uint16) []*portpkg.LocalPortResult {
	return []*portpkg.LocalPortResult{{
		Port:     port,
		Bindings: []portpkg.Binding{{Port: port, IP: "127.0.0.1", Proto: "tcp", State: "ESTABLISHED", PID: 99, Name: "curl"}},
	}}
}

// localFree는 바인딩이 없는 결과를 만듭니다.
func localFree(port uint16) []*portpkg.LocalPortResult {
	return []*portpkg.LocalPortResult{{Port: port}}
}

// assertExitCode는 조용한 종료 코드 에러를 검증합니다.
func assertExitCode(t *testing.T, err error, want int) {
	t.Helper()
	if err == nil {
		t.Fatalf("종료 코드 %d가 기대되었지만 에러가 없습니다", want)
	}
	var se *silentExitError
	if !errors.As(err, &se) {
		t.Fatalf("silentExitError를 기대했지만 실제: %v", err)
	}
	if se.code != want {
		t.Errorf("종료 코드 = %d, 기대 %d", se.code, want)
	}
}

// ─── 인자 검증 ───────────────────────────────────────────────────────────────

// TestCheckRun_NoPort_UsageError는 포트 미지정 시 사용 오류(2)를 반환하는지 검증합니다.
func TestCheckRun_NoPort_UsageError(t *testing.T) {
	app, _, _ := newCheckAppForTest(&fakeCheckOps{}, "", "")

	err := app.Run()
	var exitErr *ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != ExitCodeUsage {
		t.Fatalf("기대 ExitCodeUsage(%d), 실제: %v", ExitCodeUsage, err)
	}
}

// TestCheckRun_InvalidPort_UsageError는 잘못된 포트 입력 시 사용 오류(2)를 반환하는지 검증합니다.
func TestCheckRun_InvalidPort_UsageError(t *testing.T) {
	app, _, _ := newCheckAppForTest(&fakeCheckOps{}, "3000-2000", "")

	err := app.Run()
	var exitErr *ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != ExitCodeUsage {
		t.Fatalf("기대 ExitCodeUsage(%d), 실제: %v", ExitCodeUsage, err)
	}
}

// ─── 로컬 확인 ───────────────────────────────────────────────────────────────

// TestCheckRun_Local_Listening은 LISTEN 포트를 성공(0)으로 보고하는지 검증합니다.
func TestCheckRun_Local_Listening(t *testing.T) {
	ops := &fakeCheckOps{localFunc: func(uint16, uint16) ([]*portpkg.LocalPortResult, error) {
		return localListening(8080), nil
	}}
	app, out, errW := newCheckAppForTest(ops, "8080", "")

	if err := app.Run(); err != nil {
		t.Fatalf("Run 오류: %v", err)
	}
	s := out.String()
	for _, want := range []string{"8080", "LISTEN", "node", "1234", "PROTO"} {
		if !strings.Contains(s, want) {
			t.Errorf("출력에 %q 누락: %q", want, s)
		}
	}
	if !strings.Contains(errW.String(), "🔍") {
		t.Errorf("진행 메시지는 stderr로 가야 합니다: %q", errW.String())
	}
	if got := ops.localRanges(); len(got) != 1 || got[0] != [2]uint16{8080, 8080} {
		t.Errorf("CheckLocalRange 호출 = %v, 기대 [[8080 8080]]", got)
	}
}

// TestCheckRun_Local_FreePort는 미사용 포트에서 종료 코드 1을 반환하는지 검증합니다.
func TestCheckRun_Local_FreePort_ExitCodeError(t *testing.T) {
	ops := &fakeCheckOps{localFunc: func(uint16, uint16) ([]*portpkg.LocalPortResult, error) {
		return localFree(8080), nil
	}}
	app, out, _ := newCheckAppForTest(ops, "8080", "")

	assertExitCode(t, app.Run(), ExitCodeError)
	if !strings.Contains(out.String(), "사용 중이 아님") {
		t.Errorf("미사용 안내가 출력되어야 합니다: %q", out.String())
	}
}

// TestCheckRun_Local_EstablishedOnly는 LISTEN 아닌 바인딩만 있을 때 종료 코드 1을 반환하는지 검증합니다.
func TestCheckRun_Local_EstablishedOnly_ExitCodeError(t *testing.T) {
	ops := &fakeCheckOps{localFunc: func(uint16, uint16) ([]*portpkg.LocalPortResult, error) {
		return localEstablished(52134), nil
	}}
	app, out, _ := newCheckAppForTest(ops, "52134", "")

	assertExitCode(t, app.Run(), ExitCodeError)
	if !strings.Contains(out.String(), "LISTEN 아님") {
		t.Errorf("LISTEN 아님 안내가 출력되어야 합니다: %q", out.String())
	}
}

// TestCheckRun_Local_RangeTable는 범위 확인이 표로 출력되고 요약이 stderr로 가는지 검증합니다.
func TestCheckRun_Local_RangeTable(t *testing.T) {
	ops := &fakeCheckOps{localFunc: func(start, end uint16) ([]*portpkg.LocalPortResult, error) {
		return []*portpkg.LocalPortResult{
			{Port: start, Listening: true, Bindings: []portpkg.Binding{
				{Port: start, IP: "127.0.0.1", Proto: "tcp", State: "LISTEN", PID: 1, Name: "a"},
			}},
			{Port: end},
		}, nil
	}}
	app, out, errW := newCheckAppForTest(ops, "3000-3001", "")

	if err := app.Run(); err != nil {
		t.Fatalf("Run 오류: %v", err)
	}
	s := out.String()
	for _, want := range []string{"PORT", "PROTO", "STATE", "ADDRESS", "NAME", "3000", "a"} {
		if !strings.Contains(s, want) {
			t.Errorf("표 출력에 %q 누락: %q", want, s)
		}
	}
	if strings.Contains(s, ":3001") {
		t.Errorf("미사용 포트는 표에 나오면 안 됩니다: %q", s)
	}
	if !strings.Contains(errW.String(), "2개 포트") {
		t.Errorf("요약은 stderr로 가야 합니다: %q", errW.String())
	}
}

// TestCheckRun_Local_Range_NoOccupiedPort는 범위 전체가 미사용일 때 종료 코드 1을 반환하는지 검증합니다.
func TestCheckRun_Local_Range_NoOccupiedPort(t *testing.T) {
	ops := &fakeCheckOps{localFunc: func(start, end uint16) ([]*portpkg.LocalPortResult, error) {
		return []*portpkg.LocalPortResult{{Port: start}, {Port: end}}, nil
	}}
	app, out, _ := newCheckAppForTest(ops, "3000-3001", "")

	assertExitCode(t, app.Run(), ExitCodeError)
	if !strings.Contains(out.String(), "사용 중인 포트가 없습니다") {
		t.Errorf("범위 미사용 안내가 출력되어야 합니다: %q", out.String())
	}
}

// TestCheckRun_Local_OpsError는 소켓 조회 실패가 그대로 전파되는지 검증합니다.
func TestCheckRun_Local_OpsError(t *testing.T) {
	wantErr := errors.New("네트워크 접근 실패")
	ops := &fakeCheckOps{localFunc: func(uint16, uint16) ([]*portpkg.LocalPortResult, error) {
		return nil, wantErr
	}}
	app, _, _ := newCheckAppForTest(ops, "8080", "")

	if err := app.Run(); !errors.Is(err, wantErr) {
		t.Fatalf("원본 오류가 전파되어야 합니다: %v", err)
	}
}

// TestCheckRun_Local_JSON은 JSON 출력이 유효하고 바인딩된 포트만 포함하는지 검증합니다.
func TestCheckRun_Local_JSON(t *testing.T) {
	ops := &fakeCheckOps{localFunc: func(uint16, uint16) ([]*portpkg.LocalPortResult, error) {
		return []*portpkg.LocalPortResult{
			{Port: 8080, Listening: true, Bindings: []portpkg.Binding{
				{Port: 8080, IP: "0.0.0.0", Proto: "tcp", State: "LISTEN", PID: 1234, Name: "node"},
			}},
			{Port: 8081},
		}, nil
	}}
	app, out, errW := newCheckAppForTest(ops, "8080-8081", "")
	app.JSON = true

	if err := app.Run(); err != nil {
		t.Fatalf("Run 오류: %v", err)
	}
	if errW.Len() != 0 {
		t.Errorf("JSON 모드에서는 진행 메시지를 출력하면 안 됩니다: %q", errW.String())
	}

	var entries []struct {
		Port      uint16 `json:"port"`
		Listening bool   `json:"listening"`
		Bindings  []struct {
			Proto   string `json:"proto"`
			State   string `json:"state"`
			Address string `json:"address"`
			PID     int32  `json:"pid"`
			Name    string `json:"name"`
		} `json:"bindings"`
	}
	if err := json.Unmarshal(out.Bytes(), &entries); err != nil {
		t.Fatalf("JSON 파싱 실패: %v (출력=%q)", err, out.String())
	}
	if len(entries) != 1 {
		t.Fatalf("JSON 항목 수 = %d, 기대 1 (바인딩 없는 포트는 제외): %q", len(entries), out.String())
	}
	e := entries[0]
	if e.Port != 8080 || !e.Listening {
		t.Errorf("JSON 항목 = %+v", e)
	}
	if len(e.Bindings) != 1 || e.Bindings[0].Address != "0.0.0.0:8080" || e.Bindings[0].Name != "node" {
		t.Errorf("JSON 바인딩 = %+v", e.Bindings)
	}
}

// TestCheckRun_Local_JSON_EmptyArray는 바인딩이 없을 때 빈 배열을 출력하는지 검증합니다.
func TestCheckRun_Local_JSON_EmptyArray(t *testing.T) {
	ops := &fakeCheckOps{localFunc: func(uint16, uint16) ([]*portpkg.LocalPortResult, error) {
		return localFree(8080), nil
	}}
	app, out, _ := newCheckAppForTest(ops, "8080", "")
	app.JSON = true

	assertExitCode(t, app.Run(), ExitCodeError)
	if got := strings.TrimSpace(out.String()); got != "[]" {
		t.Errorf("JSON 출력 = %q, 기대 []", got)
	}
}

// TestCheckRun_Local_Quiet은 -q에서 진행/요약 메시지를 억제하는지 검증합니다.
func TestCheckRun_Local_Quiet(t *testing.T) {
	ops := &fakeCheckOps{localFunc: func(uint16, uint16) ([]*portpkg.LocalPortResult, error) {
		return localListening(8080), nil
	}}
	app, out, errW := newCheckAppForTest(ops, "8080", "")
	app.Quiet = true

	if err := app.Run(); err != nil {
		t.Fatalf("Run 오류: %v", err)
	}
	if errW.Len() != 0 {
		t.Errorf("quiet 모드에서 stderr는 비어야 합니다: %q", errW.String())
	}
	if strings.Contains(out.String(), "🔍") {
		t.Errorf("quiet 모드에서 진행 메시지가 출력되면 안 됩니다: %q", out.String())
	}
	if !strings.Contains(out.String(), "LISTEN") {
		t.Errorf("결과 데이터는 유지되어야 합니다: %q", out.String())
	}
}

// ─── 원격 확인 ───────────────────────────────────────────────────────────────

// TestCheckRun_Remote_Open는 원격 연결 성공을 성공(0)으로 보고하는지 검증합니다.
func TestCheckRun_Remote_Open(t *testing.T) {
	ops := &fakeCheckOps{remoteFunc: func(host string, port uint16) *portpkg.ReachResult {
		return &portpkg.ReachResult{Host: host, Port: port, Status: portpkg.ReachOpen, Latency: 3 * time.Millisecond}
	}}
	app, out, errW := newCheckAppForTest(ops, "8080", "192.168.0.10")

	if err := app.Run(); err != nil {
		t.Fatalf("Run 오류: %v", err)
	}
	s := out.String()
	if !strings.Contains(s, "192.168.0.10:8080") || !strings.Contains(s, "open") {
		t.Errorf("도달성 결과가 출력되어야 합니다: %q", s)
	}
	if !strings.Contains(errW.String(), "🔍") {
		t.Errorf("진행 메시지는 stderr로 가야 합니다: %q", errW.String())
	}
}

// TestCheckRun_Remote_Closed는 연결 거부 시 종료 코드 1을 반환하는지 검증합니다.
func TestCheckRun_Remote_Closed_ExitCodeError(t *testing.T) {
	ops := &fakeCheckOps{remoteFunc: func(host string, port uint16) *portpkg.ReachResult {
		return &portpkg.ReachResult{Host: host, Port: port, Status: portpkg.ReachClosed, Detail: "연결이 거부되었습니다"}
	}}
	app, out, _ := newCheckAppForTest(ops, "8080", "192.168.0.10")

	assertExitCode(t, app.Run(), ExitCodeError)
	if !strings.Contains(out.String(), "closed") {
		t.Errorf("closed 결과가 출력되어야 합니다: %q", out.String())
	}
}

// TestCheckRun_Remote_JSON은 원격 JSON 출력이 유효한지 검증합니다.
func TestCheckRun_Remote_JSON(t *testing.T) {
	ops := &fakeCheckOps{remoteFunc: func(host string, port uint16) *portpkg.ReachResult {
		return &portpkg.ReachResult{Host: host, Port: port, Status: portpkg.ReachFiltered, Latency: 5 * time.Millisecond, Detail: "응답 없음"}
	}}
	app, out, errW := newCheckAppForTest(ops, "8080", "10.0.0.1")
	app.JSON = true

	assertExitCode(t, app.Run(), ExitCodeError)
	if errW.Len() != 0 {
		t.Errorf("JSON 모드에서는 진행 메시지를 출력하면 안 됩니다: %q", errW.String())
	}
	if !json.Valid(out.Bytes()) {
		t.Fatalf("유효한 JSON이 아닙니다: %q", out.String())
	}
	var entries []struct {
		Host      string `json:"host"`
		Port      uint16 `json:"port"`
		Status    string `json:"status"`
		Open      bool   `json:"open"`
		LatencyMs int64  `json:"latency_ms"`
		Detail    string `json:"detail"`
	}
	if err := json.Unmarshal(out.Bytes(), &entries); err != nil {
		t.Fatalf("JSON 파싱 실패: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("JSON 항목 수 = %d, 기대 1", len(entries))
	}
	if entries[0].Status != "filtered" || entries[0].Open || entries[0].LatencyMs != 5 {
		t.Errorf("JSON 항목 = %+v", entries[0])
	}
}

// TestCheckRun_Remote_Range_AllPortsDialedOnce는 범위 확인이 모든 포트를 한 번씩 병렬 확인하는지 검증합니다.
func TestCheckRun_Remote_Range_AllPortsDialedOnce(t *testing.T) {
	ops := &fakeCheckOps{remoteFunc: func(host string, port uint16) *portpkg.ReachResult {
		return &portpkg.ReachResult{Host: host, Port: port, Status: portpkg.ReachClosed}
	}}
	app, out, errW := newCheckAppForTest(ops, "8000-8010", "10.0.0.1")

	assertExitCode(t, app.Run(), ExitCodeError)

	dialed := ops.dialedPorts()
	if len(dialed) != 11 {
		t.Fatalf("Dial 횟수 = %d, 기대 11: %v", len(dialed), dialed)
	}
	seen := make(map[string]int)
	for _, d := range dialed {
		seen[d]++
	}
	for p := 8000; p <= 8010; p++ {
		key := fmt.Sprintf("10.0.0.1:%d", p)
		if seen[key] != 1 {
			t.Errorf("%s Dial 횟수 = %d, 기대 1", key, seen[key])
		}
	}
	// 결과는 포트 오름차순으로 출력되어야 합니다.
	s := out.String()
	if i, j := strings.Index(s, ":8000"), strings.Index(s, ":8010"); i < 0 || j < 0 || i > j {
		t.Errorf("결과가 포트 오름차순이 아닙니다 (8000=%d, 8010=%d): %q", i, j, s)
	}
	_ = errW
}

// TestCheckRun_Remote_Range_AnyOpenIsSuccess는 범위 중 하나라도 열려 있으면 성공(0)인지 검증합니다.
func TestCheckRun_Remote_Range_AnyOpenIsSuccess(t *testing.T) {
	ops := &fakeCheckOps{remoteFunc: func(host string, port uint16) *portpkg.ReachResult {
		status := portpkg.ReachClosed
		if port == 8005 {
			status = portpkg.ReachOpen
		}
		return &portpkg.ReachResult{Host: host, Port: port, Status: status}
	}}
	app, out, _ := newCheckAppForTest(ops, "8000-8010", "10.0.0.1")

	if err := app.Run(); err != nil {
		t.Fatalf("열린 포트가 있으면 성공해야 합니다: %v", err)
	}
	if !strings.Contains(out.String(), "8005") {
		t.Errorf("열린 포트가 출력되어야 합니다: %q", out.String())
	}
}

// TestCheckRun_Remote_Quiet은 -q에서 진행/요약 메시지를 억제하는지 검증합니다.
func TestCheckRun_Remote_Quiet(t *testing.T) {
	ops := &fakeCheckOps{}
	app, out, errW := newCheckAppForTest(ops, "8080", "10.0.0.1")
	app.Quiet = true

	if err := app.Run(); err != nil {
		t.Fatalf("Run 오류: %v", err)
	}
	if errW.Len() != 0 {
		t.Errorf("quiet 모드에서 stderr는 비어야 합니다: %q", errW.String())
	}
	if strings.Contains(out.String(), "🔍") {
		t.Errorf("quiet 모드에서 진행 메시지가 출력되면 안 됩니다: %q", out.String())
	}
}

// TestCheckRun_Remote_RangeSummaryOnStderr는 범위 요약이 stderr(요약)로 출력되는지 검증합니다.
func TestCheckRun_Remote_RangeSummaryOnStderr(t *testing.T) {
	ops := &fakeCheckOps{remoteFunc: func(host string, port uint16) *portpkg.ReachResult {
		status := portpkg.ReachClosed
		if port == 8000 {
			status = portpkg.ReachOpen
		}
		return &portpkg.ReachResult{Host: host, Port: port, Status: status}
	}}
	app, _, errW := newCheckAppForTest(ops, "8000-8001", "10.0.0.1")

	if err := app.Run(); err != nil {
		t.Fatalf("Run 오류: %v", err)
	}
	if !strings.Contains(errW.String(), "2개 포트") || !strings.Contains(errW.String(), "1개") {
		t.Errorf("요약은 stderr로 가야 합니다: %q", errW.String())
	}
}

// ─── 실제 앱 구성 ────────────────────────────────────────────────────────────

// TestNewRealCheckApp_Defaults는 실제 앱 구성이 기본 타임아웃/플래그를 반영하는지 검증합니다.
func TestNewRealCheckApp_Defaults(t *testing.T) {
	app := newRealCheckApp("8080", "10.0.0.1", 0, true, true)

	if app.PortStr != "8080" || app.Host != "10.0.0.1" {
		t.Errorf("PortStr/Host = %q/%q", app.PortStr, app.Host)
	}
	if app.Timeout != portpkg.DefaultCheckTimeout {
		t.Errorf("Timeout = %v, 기대 기본값 %v", app.Timeout, portpkg.DefaultCheckTimeout)
	}
	if !app.JSON || !app.Quiet {
		t.Error("JSON/Quiet 플래그가 반영되어야 합니다")
	}
	if app.Ops == nil {
		t.Error("실제 Checker가 주입되어야 합니다")
	}
}

// TestNewRealCheckApp_CustomTimeout은 지정한 타임아웃이 유지되는지 검증합니다.
func TestNewRealCheckApp_CustomTimeout(t *testing.T) {
	app := newRealCheckApp("8080", "", 700*time.Millisecond, false, false)

	if app.Timeout != 700*time.Millisecond {
		t.Errorf("Timeout = %v, 기대 700ms", app.Timeout)
	}
}
