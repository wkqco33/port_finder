//go:build !integration

// 이 파일은 OS에 의존하지 않는 빠른 결정적 단위 테스트입니다.
// 페이크를 주입하여 항상 실행되며 (`go test ./...`),
// 실제 OS에 의존하는 통합 테스트는 `-tags integration`로 별도 실행합니다 (port_test.go).
package port_test

import (
	"fmt"
	"testing"
	"time"

	portpkg "port_finder/pkg/port"

	"github.com/shirou/gopsutil/v4/net"
)

// ─── 페이크 구현 ──────────────────────────────────────────────────────────────

// fakeConn은 ConnectionSource를 대체하는 페이크입니다.
type fakeConn struct {
	conns []net.ConnectionStat
	err   error
}

func (f *fakeConn) Connections(kind string) ([]net.ConnectionStat, error) {
	return f.conns, f.err
}

// fakeProcess는 port.Process를 대체하는 페이크입니다.
type fakeProcess struct {
	name    string
	nameErr error

	killErr error
	termErr error
	running bool
	runErr  error

	killed     bool
	terminated bool
}

func (f *fakeProcess) Name() (string, error) { return f.name, f.nameErr }
func (f *fakeProcess) Kill() error           { f.killed = true; return f.killErr }
func (f *fakeProcess) Terminate() error      { f.terminated = true; return f.termErr }
func (f *fakeProcess) IsRunning() (bool, error) {
	return f.running, f.runErr
}

// fakeProcSource는 PID → 프로세스 조회를 대체하는 페이크입니다.
type fakeProcSource struct {
	procs map[int32]*fakeProcess
	err   error
}

func (f *fakeProcSource) NewProcess(pid int32) (portpkg.Process, error) {
	if f.err != nil {
		return nil, f.err
	}
	if p, ok := f.procs[pid]; ok {
		return p, nil
	}
	return nil, fmt.Errorf("프로세스 %d 없음", pid)
}

func conn(port uint32, pid int32) net.ConnectionStat {
	return net.ConnectionStat{
		Pid:   pid,
		Laddr: net.Addr{IP: "127.0.0.1", Port: port},
	}
}

func newFinderWith(conns []net.ConnectionStat, connErr error, procs *fakeProcSource) *portpkg.Finder {
	return portpkg.NewFinder(
		portpkg.WithConnectionSource(&fakeConn{conns: conns, err: connErr}),
		portpkg.WithProcessSource(procs),
	)
}

// ─── FindByPortRange ─────────────────────────────────────────────────────────

// TestFindByPortRange_DedupesAndFilters는 범위 밖/무효 PID/중복 항목을 걸러내는지 검증합니다.
func TestFindByPortRange_DedupesAndFilters(t *testing.T) {
	conns := []net.ConnectionStat{
		conn(3000, 111),
		conn(3000, 111), // 중복 — 제외
		conn(3001, 222),
		conn(4000, 333), // 범위 밖 — 제외
		conn(3002, 0),   // PID 0 — 제외
	}
	finder := newFinderWith(conns, nil, &fakeProcSource{
		procs: map[int32]*fakeProcess{111: {name: "node"}, 222: {name: "go"}},
	})

	got, err := finder.FindByPortRange(3000, 3005)
	if err != nil {
		t.Fatalf("FindByPortRange 오류: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("결과 개수 = %d, 기대 2: %+v", len(got), got)
	}
	byPort := map[uint16]portpkg.ProcessInfo{}
	for _, r := range got {
		byPort[r.Port] = *r
	}
	if byPort[3000].Name != "node" || byPort[3000].PID != 111 {
		t.Errorf("3000 결과 = %+v", byPort[3000])
	}
	if byPort[3001].Name != "go" {
		t.Errorf("3001 결과 = %+v", byPort[3001])
	}
}

// TestFindByPortRange_Sorted는 결과가 포트 오름차순으로 정렬되는지 검증합니다.
func TestFindByPortRange_Sorted(t *testing.T) {
	conns := []net.ConnectionStat{conn(9000, 1), conn(1000, 2), conn(5000, 3)}
	finder := newFinderWith(conns, nil, &fakeProcSource{
		procs: map[int32]*fakeProcess{1: {name: "a"}, 2: {name: "b"}, 3: {name: "c"}},
	})

	got, err := finder.FindByPortRange(1, 65535)
	if err != nil {
		t.Fatalf("FindByPortRange 오류: %v", err)
	}
	for i := 1; i < len(got); i++ {
		if got[i].Port < got[i-1].Port {
			t.Errorf("정렬 위반: got[%d]=%d < got[%d]=%d", i, got[i].Port, i-1, got[i-1].Port)
		}
	}
}

// TestFindByPortRange_ConnectionError는 연결 소스 오류가 전파되는지 검증합니다.
func TestFindByPortRange_ConnectionError(t *testing.T) {
	wantErr := fmt.Errorf("네트워크 접근 실패")
	finder := newFinderWith(nil, wantErr, &fakeProcSource{})

	if _, err := finder.FindByPortRange(1, 10); err == nil {
		t.Fatal("연결 소스 오류가 전파되어야 합니다")
	}
}

// TestFindByPortRange_ProcessNameFallback은 프로세스 이름 조회 실패 시 "unknown"을 반환하는지 검증합니다.
func TestFindByPortRange_ProcessNameFallback(t *testing.T) {
	conns := []net.ConnectionStat{conn(3000, 111)}
	finder := newFinderWith(conns, nil, &fakeProcSource{
		procs: map[int32]*fakeProcess{111: {nameErr: fmt.Errorf("조회 실패")}},
	})

	got, err := finder.FindByPortRange(3000, 3000)
	if err != nil {
		t.Fatalf("FindByPortRange 오류: %v", err)
	}
	if len(got) != 1 || got[0].Name != "unknown" {
		t.Errorf("이름 폴백 결과 = %+v, 기대 name=unknown", got)
	}
}

// TestFindByPort_DelegatesToRange는 FindByPort가 단일 포트로 FindByPortRange를 호출하는지 검증합니다.
func TestFindByPort_DelegatesToRange(t *testing.T) {
	conns := []net.ConnectionStat{conn(8080, 42)}
	finder := newFinderWith(conns, nil, &fakeProcSource{
		procs: map[int32]*fakeProcess{42: {name: "server"}},
	})

	got, err := finder.FindByPort(8080)
	if err != nil {
		t.Fatalf("FindByPort 오류: %v", err)
	}
	if len(got) != 1 || got[0].Port != 8080 {
		t.Errorf("FindByPort 결과 = %+v", got)
	}
}

// TestListAll_ReturnsAllPorts는 ListAll이 전체 포트를 스캔하는지 검증합니다.
func TestListAll_ReturnsAllPorts(t *testing.T) {
	conns := []net.ConnectionStat{conn(22, 1), conn(8080, 2)}
	finder := newFinderWith(conns, nil, &fakeProcSource{
		procs: map[int32]*fakeProcess{1: {name: "a"}, 2: {name: "b"}},
	})

	got, err := finder.ListAll()
	if err != nil {
		t.Fatalf("ListAll 오류: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("ListAll 결과 개수 = %d, 기대 2", len(got))
	}
}

// ─── KillProcessByPID ─────────────────────────────────────────────────────────

// TestKillProcessByPID_NotFound는 존재하지 않는 PID에 대해 오류를 반환하는지 검증합니다.
func TestKillProcessByPID_NotFound(t *testing.T) {
	finder := newFinderWith(nil, nil, &fakeProcSource{})
	if err := finder.KillProcessByPID(999); err == nil {
		t.Error("존재하지 않는 PID에 대해 오류가 반환되어야 합니다")
	}
}

// TestKillProcessByPID_CallsKill은 종료 호출이 프로세스로 전달되는지 검증합니다.
func TestKillProcessByPID_CallsKill(t *testing.T) {
	p := &fakeProcess{}
	finder := newFinderWith(nil, nil, &fakeProcSource{procs: map[int32]*fakeProcess{7: p}})
	if err := finder.KillProcessByPID(7); err != nil {
		t.Fatalf("KillProcessByPID 오류: %v", err)
	}
	if !p.killed {
		t.Error("KillProcessByPID가 Kill()을 호출해야 합니다")
	}
}

// ─── KillProcessGracefully ────────────────────────────────────────────────────

// TestKillProcessGracefully_TerminateOk_StopsRunning은 SIGTERM 후 종료 확인을 검증합니다.
func TestKillProcessGracefully_TerminateOk_StopsRunning(t *testing.T) {
	p := &fakeProcess{}
	finder := newFinderWith(nil, nil, &fakeProcSource{procs: map[int32]*fakeProcess{7: p}})

	if err := finder.KillProcessGracefully(7, 500*time.Millisecond); err != nil {
		t.Fatalf("KillProcessGracefully 오류: %v", err)
	}
}

// TestKillProcessGracefully_TerminateErr_EscalatesToKill는 Terminate 실패 시 Kill로 에스컬레이션하는지 검증합니다.
func TestKillProcessGracefully_TerminateErr_EscalatesToKill(t *testing.T) {
	termErr := fmt.Errorf("terminate 실패")
	p := &fakeProcess{termErr: termErr}
	finder := newFinderWith(nil, nil, &fakeProcSource{procs: map[int32]*fakeProcess{7: p}})

	if err := finder.KillProcessGracefully(7, 100*time.Millisecond); err != nil {
		t.Fatalf("KillProcessGracefully 오류: %v", err)
	}
}

// TestKillProcessGracefully_NotFound는 존재하지 않는 PID에 대해 오류를 반환하는지 검증합니다.
func TestKillProcessGracefully_NotFound(t *testing.T) {
	finder := newFinderWith(nil, nil, &fakeProcSource{})
	if err := finder.KillProcessGracefully(999, 100*time.Millisecond); err == nil {
		t.Error("존재하지 않는 PID에 대해 오류가 반환되어야 합니다")
	}
}
