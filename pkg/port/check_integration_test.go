//go:build integration

// 이 파일은 실제 OS 소켓/네트워크를 사용하는 통합 테스트입니다.
// 기본 `go test ./...`에서는 제외되며 `go test -tags integration`으로 실행합니다.
// 빠른 결정적 단위 테스트는 check_unit_test.go를 참고하세요.
package port_test

import (
	"context"
	"os"
	"testing"
	"time"

	portpkg "port_finder/pkg/port"
)

// ─── CheckLocal (실제 소켓) ──────────────────────────────────────────────────

// TestCheckLocal_RealListenerIsListening는 실제로 연 리스너를 LISTEN으로 탐지하는지 검증합니다.
func TestCheckLocal_RealListenerIsListening(t *testing.T) {
	ln, port := listenTCP(t)
	defer ln.Close()
	time.Sleep(50 * time.Millisecond)

	res, err := portpkg.CheckLocal(port)
	if err != nil {
		t.Fatalf("CheckLocal 오류: %v", err)
	}
	if !res.Listening {
		t.Fatalf("포트 %d가 Listening으로 탐지되지 않았습니다: %+v", port, res.Bindings)
	}

	wantPID := int32(os.Getpid())
	found := false
	for _, b := range res.Bindings {
		if b.PID != wantPID {
			continue
		}
		found = true
		if b.Proto != "tcp" {
			t.Errorf("Proto = %q, 기대 tcp", b.Proto)
		}
		if b.State != "LISTEN" {
			t.Errorf("State = %q, 기대 LISTEN", b.State)
		}
	}
	if !found {
		t.Errorf("현재 프로세스(PID %d)의 바인딩을 찾지 못했습니다: %+v", wantPID, res.Bindings)
	}
}

// TestCheckLocal_ClosedListenerIsFree는 닫은 포트를 미사용으로 판정하는지 검증합니다.
func TestCheckLocal_ClosedListenerIsFree(t *testing.T) {
	ln, port := listenTCP(t)
	ln.Close()
	time.Sleep(50 * time.Millisecond)

	res, err := portpkg.CheckLocal(port)
	if err != nil {
		t.Fatalf("CheckLocal 오류: %v", err)
	}
	if res.Listening {
		t.Errorf("닫힌 포트 %d가 Listening으로 탐지되었습니다: %+v", port, res.Bindings)
	}
}

// TestCheckLocal_SelfIsListening는 자기 자신(루프백)을 원격 확인으로도 open 판정하는지 검증합니다.
func TestCheckLocal_SelfIsListening(t *testing.T) {
	ln, port := listenTCP(t)
	defer ln.Close()

	res := portpkg.CheckRemote(context.Background(), "127.0.0.1", port)
	if res.Status != portpkg.ReachOpen {
		t.Fatalf("127.0.0.1:%d Status = %q (detail=%s), 기대 open", port, res.Status, res.Detail)
	}
}

// TestCheckRemote_ClosedPortIsClosed는 실제 닫힌 포트가 closed로 판정되는지 검증합니다.
func TestCheckRemote_ClosedPortIsClosed(t *testing.T) {
	ln, port := listenTCP(t)
	ln.Close()
	time.Sleep(50 * time.Millisecond)

	res := portpkg.CheckRemote(context.Background(), "127.0.0.1", port)
	if res.Status != portpkg.ReachClosed {
		t.Fatalf("닫힌 포트 %d Status = %q (detail=%s), 기대 closed", port, res.Status, res.Detail)
	}
}
