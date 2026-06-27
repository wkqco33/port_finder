package port_test

import (
	"net"
	"os"
	"os/exec"
	"runtime"
	"testing"
	"time"

	portpkg "port_finder/pkg/port"
)

// listenTCP는 임의의 포트로 TCP 리스너를 열고, 실제 바인딩된 포트 번호를 반환합니다.
func listenTCP(t *testing.T) (net.Listener, uint16) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("TCP 리스너를 열 수 없습니다: %v", err)
	}
	port := uint16(ln.Addr().(*net.TCPAddr).Port)
	return ln, port
}

// sleepCmd은 플랫폼에 맞는 sleep 명령을 반환합니다.
func sleepCmd() (string, []string) {
	if runtime.GOOS == "windows" {
		return "timeout", []string{"/t", "100"}
	}
	return "sleep", []string{"100"}
}

// ─── FindByPort ───────────────────────────────────────────────────────────────

// TestFindByPort_Found는 현재 프로세스가 열어둔 포트를 FindByPort가 올바르게 탐지하는지 검증합니다.
func TestFindByPort_Found(t *testing.T) {
	ln, port := listenTCP(t)
	defer ln.Close()

	time.Sleep(50 * time.Millisecond)

	infos, err := portpkg.FindByPort(port)
	if err != nil {
		t.Fatalf("FindByPort 오류: %v", err)
	}
	if len(infos) == 0 {
		t.Fatalf("포트 %d를 사용하는 프로세스를 찾지 못했습니다 (예상: 현재 프로세스)", port)
	}

	wantPID := int32(os.Getpid())
	found := false
	for _, info := range infos {
		if info.PID == wantPID {
			found = true
			if info.Port != port {
				t.Errorf("Port = %d, 기대값 = %d", info.Port, port)
			}
			if info.Name == "" {
				t.Error("프로세스 이름이 비어 있습니다")
			}
		}
	}
	if !found {
		t.Errorf("예상되는 PID %d를 찾을 수 없습니다. 결과: %+v", wantPID, infos)
	}
}

// TestFindByPort_NotFound는 사용하지 않는 포트에 대해 빈 결과를 반환하는지 검증합니다.
func TestFindByPort_NotFound(t *testing.T) {
	ln, port := listenTCP(t)
	ln.Close()
	time.Sleep(50 * time.Millisecond)

	infos, err := portpkg.FindByPort(port)
	if err != nil {
		t.Fatalf("FindByPort 오류: %v", err)
	}
	if len(infos) != 0 {
		t.Errorf("사용하지 않는 포트 %d에서 프로세스가 감지됨: %+v", port, infos)
	}
}

// ─── FindByPortRange ──────────────────────────────────────────────────────────

// TestFindByPortRange_MultipleResults는 범위 스캔이 여러 포트를 올바르게 반환하는지 검증합니다.
func TestFindByPortRange_MultipleResults(t *testing.T) {
	ln1, port1 := listenTCP(t)
	defer ln1.Close()
	ln2, port2 := listenTCP(t)
	defer ln2.Close()

	lo, hi := port1, port2
	if lo > hi {
		lo, hi = hi, lo
	}

	time.Sleep(50 * time.Millisecond)

	results, err := portpkg.FindByPortRange(lo, hi)
	if err != nil {
		t.Fatalf("FindByPortRange 오류: %v", err)
	}

	found := make(map[uint16]bool)
	for _, r := range results {
		found[r.Port] = true
	}
	if !found[port1] {
		t.Errorf("포트 %d가 범위 스캔 결과에 없습니다", port1)
	}
	if !found[port2] {
		t.Errorf("포트 %d가 범위 스캔 결과에 없습니다", port2)
	}
}

// TestFindByPortRange_Sorted는 결과가 포트 번호 오름차순으로 정렬되는지 검증합니다.
func TestFindByPortRange_Sorted(t *testing.T) {
	ln1, port1 := listenTCP(t)
	defer ln1.Close()
	ln2, port2 := listenTCP(t)
	defer ln2.Close()

	lo, hi := port1, port2
	if lo > hi {
		lo, hi = hi, lo
	}

	time.Sleep(50 * time.Millisecond)

	results, err := portpkg.FindByPortRange(lo, hi)
	if err != nil {
		t.Fatalf("FindByPortRange 오류: %v", err)
	}

	for i := 1; i < len(results); i++ {
		if results[i].Port < results[i-1].Port {
			t.Errorf("결과가 정렬되지 않았습니다: 인덱스 %d (%d) < 인덱스 %d (%d)",
				i, results[i].Port, i-1, results[i-1].Port)
		}
	}
}

// ─── ListAll ──────────────────────────────────────────────────────────────────

// TestListAll_NotEmpty는 활성 리스너가 있을 때 ListAll이 결과를 반환하는지 검증합니다.
func TestListAll_NotEmpty(t *testing.T) {
	ln, _ := listenTCP(t)
	defer ln.Close()

	time.Sleep(50 * time.Millisecond)

	results, err := portpkg.ListAll()
	if err != nil {
		t.Fatalf("ListAll 오류: %v", err)
	}
	if len(results) == 0 {
		t.Error("ListAll이 빈 결과를 반환했습니다 (활성 포트가 있는데도)")
	}
}

// ─── KillProcessByPID ────────────────────────────────────────────────────────

// TestKillProcessByPID는 서브프로세스를 생성하고 KillProcessByPID로 정상 종료되는지 검증합니다.
func TestKillProcessByPID(t *testing.T) {
	cmd, args := sleepCmd()
	proc := exec.Command(cmd, args...)
	if err := proc.Start(); err != nil {
		t.Fatalf("서브프로세스 시작 실패: %v", err)
	}

	pid := int32(proc.Process.Pid)
	if err := portpkg.KillProcessByPID(pid); err != nil {
		t.Fatalf("KillProcessByPID(%d) 오류: %v", pid, err)
	}

	done := make(chan error, 1)
	go func() { done <- proc.Wait() }()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Error("프로세스가 3초 안에 종료되지 않았습니다")
	}
}

// TestKillProcessByPID_InvalidPID는 존재하지 않는 PID에 대해 오류를 반환하는지 검증합니다.
func TestKillProcessByPID_InvalidPID(t *testing.T) {
	if err := portpkg.KillProcessByPID(0); err == nil {
		t.Error("유효하지 않은 PID 0에 대해 오류가 반환되어야 합니다")
	}
}

// ─── KillProcessGracefully ───────────────────────────────────────────────────

// TestKillProcessGracefully는 Graceful 종료가 프로세스를 정상 종료시키는지 검증합니다.
func TestKillProcessGracefully(t *testing.T) {
	cmd, args := sleepCmd()
	proc := exec.Command(cmd, args...)
	if err := proc.Start(); err != nil {
		t.Fatalf("서브프로세스 시작 실패: %v", err)
	}

	pid := int32(proc.Process.Pid)
	if err := portpkg.KillProcessGracefully(pid, 3*time.Second); err != nil {
		t.Fatalf("KillProcessGracefully(%d) 오류: %v", pid, err)
	}

	done := make(chan error, 1)
	go func() { done <- proc.Wait() }()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Error("Graceful 종료 후 프로세스가 5초 안에 종료되지 않았습니다")
	}
}
