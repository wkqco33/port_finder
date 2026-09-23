//go:build !integration

// 이 파일은 Checker(로컬 바인딩 확인 / 원격 TCP 도달성 확인)의 hermetic 단위 테스트입니다.
// 페이크 소켓 소스와 페이크 Dialer를 주입해 실제 네트워크/OS 없이 결정적으로 검증합니다.
// 실제 소켓을 사용하는 통합 테스트는 check_integration_test.go를 참고하세요.
package port_test

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	portpkg "port_finder/pkg/port"

	gonet "github.com/shirou/gopsutil/v4/net"
)

// ─── 페이크 구현 ──────────────────────────────────────────────────────────────

// connFull은 상태/소켓 타입/패밀리까지 지정할 수 있는 ConnectionStat 페이크를 만듭니다.
func connFull(port uint32, pid int32, ip, status string, sockType, family uint32) gonet.ConnectionStat {
	return gonet.ConnectionStat{
		Pid:    pid,
		Laddr:  gonet.Addr{IP: ip, Port: port},
		Status: status,
		Type:   sockType,
		Family: family,
	}
}

// procsFor는 conns에 등장하는 PID마다 "proc-<PID>" 이름을 돌려주는 페이크 소스를 만듭니다.
func procsFor(conns []gonet.ConnectionStat) *fakeProcSource {
	procs := make(map[int32]*fakeProcess)
	for _, c := range conns {
		if c.Pid <= 0 {
			continue
		}
		if _, ok := procs[c.Pid]; ok {
			continue
		}
		procs[c.Pid] = &fakeProcess{name: fmt.Sprintf("proc-%d", c.Pid)}
	}
	return &fakeProcSource{procs: procs}
}

// newCheckerWith는 페이크 연결 소스를 가진 Finder를 주입한 Checker를 만듭니다.
func newCheckerWith(conns []gonet.ConnectionStat, connErr error, opts ...portpkg.CheckOption) *portpkg.Checker {
	base := []portpkg.CheckOption{
		portpkg.WithFinder(newFinderWith(conns, connErr, procsFor(conns))),
	}
	return portpkg.NewChecker(append(base, opts...)...)
}

// fakeDialConn은 Dialer가 반환하는 최소 net.Conn 구현입니다 (연결 닫힘 여부만 추적).
type fakeDialConn struct {
	closed bool
}

func (c *fakeDialConn) Read([]byte) (int, error)         { return 0, nil }
func (c *fakeDialConn) Write(b []byte) (int, error)      { return len(b), nil }
func (c *fakeDialConn) Close() error                     { c.closed = true; return nil }
func (c *fakeDialConn) LocalAddr() net.Addr              { return fakeAddr("127.0.0.1:0") }
func (c *fakeDialConn) RemoteAddr() net.Addr             { return fakeAddr("127.0.0.1:0") }
func (c *fakeDialConn) SetDeadline(time.Time) error      { return nil }
func (c *fakeDialConn) SetReadDeadline(time.Time) error  { return nil }
func (c *fakeDialConn) SetWriteDeadline(time.Time) error { return nil }

type fakeAddr string

func (a fakeAddr) Network() string { return "tcp" }
func (a fakeAddr) String() string  { return string(a) }

// dialOutcome은 주소별 Dial 결과입니다.
type dialOutcome struct {
	conn net.Conn
	err  error
}

// fakeDialer는 Dialer를 대체하는 페이크입니다. 호출 주소/컨텍스트를 기록합니다.
type fakeDialer struct {
	mu       sync.Mutex
	outcomes map[string]dialOutcome
	calls    []string
	lastCtx  context.Context
	lastAddr string
}

func (f *fakeDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	f.mu.Lock()
	f.calls = append(f.calls, address)
	f.lastCtx = ctx
	f.lastAddr = address
	outcome := f.outcomes[address]
	f.mu.Unlock()

	if outcome.err != nil {
		return nil, outcome.err
	}
	if outcome.conn != nil {
		return outcome.conn, nil
	}
	return &fakeDialConn{}, nil
}

func (f *fakeDialer) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

func (f *fakeDialer) dialed() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.calls...)
}

// lastDeadline은 마지막 DialContext에 전달된 컨텍스트의 마감 시각을 반환합니다.
func (f *fakeDialer) lastDeadline() (time.Time, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.lastCtx == nil {
		return time.Time{}, false
	}
	return f.lastCtx.Deadline()
}

// fakeTimeoutErr은 net.Error의 Timeout()을 만족하는 페이크 오류입니다.
type fakeTimeoutErr struct{}

func (fakeTimeoutErr) Error() string   { return "i/o timeout" }
func (fakeTimeoutErr) Timeout() bool   { return true }
func (fakeTimeoutErr) Temporary() bool { return true }

// newRemoteChecker는 페이크 Dialer를 주입한 Checker를 만듭니다 (로컬 소스는 빈 페이크).
func newRemoteChecker(dialer portpkg.Dialer, opts ...portpkg.CheckOption) *portpkg.Checker {
	base := []portpkg.CheckOption{
		portpkg.WithFinder(newFinderWith(nil, nil, &fakeProcSource{})),
		portpkg.WithDialer(dialer),
	}
	return portpkg.NewChecker(append(base, opts...)...)
}

// ─── CheckLocal ──────────────────────────────────────────────────────────────

// TestCheckLocal_TCPListenIsListening는 TCP LISTEN 바인딩을 수신 대기로 판정하는지 검증합니다.
func TestCheckLocal_TCPListenIsListening(t *testing.T) {
	conns := []gonet.ConnectionStat{
		connFull(8080, 42, "0.0.0.0", "LISTEN", uint32(syscall.SOCK_STREAM), uint32(syscall.AF_INET)),
	}
	c := newCheckerWith(conns, nil)

	res, err := c.CheckLocal(8080)
	if err != nil {
		t.Fatalf("CheckLocal 오류: %v", err)
	}
	if !res.Listening {
		t.Error("LISTEN 바인딩은 Listening=true여야 합니다")
	}
	if len(res.Bindings) != 1 {
		t.Fatalf("바인딩 개수 = %d, 기대 1: %+v", len(res.Bindings), res.Bindings)
	}
	b := res.Bindings[0]
	if b.Proto != "tcp" || b.State != "LISTEN" || b.PID != 42 || b.Name != "proc-42" {
		t.Errorf("바인딩 상세 = %+v", b)
	}
	if b.Address() != "0.0.0.0:8080" {
		t.Errorf("Address() = %q, 기대 0.0.0.0:8080", b.Address())
	}
	if !b.IsListening() {
		t.Error("Binding.IsListening() = false, 기대 true")
	}
}

// TestCheckLocal_UDPBindingIsListening는 UDP는 바인딩 자체가 수신 대기임을 검증합니다.
func TestCheckLocal_UDPBindingIsListening(t *testing.T) {
	conns := []gonet.ConnectionStat{
		connFull(5353, 7, "::", "", uint32(syscall.SOCK_DGRAM), uint32(syscall.AF_INET6)),
	}
	c := newCheckerWith(conns, nil)

	res, err := c.CheckLocal(5353)
	if err != nil {
		t.Fatalf("CheckLocal 오류: %v", err)
	}
	if !res.Listening {
		t.Error("UDP 바인딩은 Listening=true여야 합니다")
	}
	if got := res.Bindings[0].Proto; got != "udp6" {
		t.Errorf("Proto = %q, 기대 udp6", got)
	}
}

// TestCheckLocal_EstablishedIsNotListening는 LISTEN 아닌 바인딩을 수신 대기로 보지 않는지 검증합니다.
func TestCheckLocal_EstablishedIsNotListening(t *testing.T) {
	conns := []gonet.ConnectionStat{
		connFull(52134, 9, "::1", "ESTABLISHED", uint32(syscall.SOCK_STREAM), uint32(syscall.AF_INET6)),
	}
	c := newCheckerWith(conns, nil)

	res, err := c.CheckLocal(52134)
	if err != nil {
		t.Fatalf("CheckLocal 오류: %v", err)
	}
	if res.Listening {
		t.Error("ESTABLISHED 바인딩은 Listening=false여야 합니다")
	}
	if len(res.Bindings) != 1 {
		t.Fatalf("바인딩은 유지되어야 합니다: %+v", res.Bindings)
	}
	if got := res.Bindings[0].Proto; got != "tcp6" {
		t.Errorf("Proto = %q, 기대 tcp6", got)
	}
	if got := res.Bindings[0].Address(); got != "[::1]:52134" {
		t.Errorf("IPv6 Address() = %q, 기대 [::1]:52134", got)
	}
}

// TestCheckLocal_FreePort는 바인딩이 없는 포트를 미사용으로 판정하는지 검증합니다.
func TestCheckLocal_FreePort(t *testing.T) {
	c := newCheckerWith(nil, nil)

	res, err := c.CheckLocal(8080)
	if err != nil {
		t.Fatalf("CheckLocal 오류: %v", err)
	}
	if res.Listening {
		t.Error("바인딩이 없으면 Listening=false여야 합니다")
	}
	if len(res.Bindings) != 0 {
		t.Errorf("바인딩 개수 = %d, 기대 0", len(res.Bindings))
	}
}

// TestCheckLocal_PidZeroNameUnknown는 PID가 없는 커널 소켓도 이름만 unknown으로 포함하는지 검증합니다.
func TestCheckLocal_PidZeroNameUnknown(t *testing.T) {
	conns := []gonet.ConnectionStat{
		connFull(80, 0, "0.0.0.0", "LISTEN", uint32(syscall.SOCK_STREAM), uint32(syscall.AF_INET)),
	}
	// PID 0에 대해 프로세스 조회를 시도하면 안 되므로, 조회 자체가 실패하는 소스를 주입합니다.
	finder := portpkg.NewFinder(
		portpkg.WithConnectionSource(&fakeConn{conns: conns}),
		portpkg.WithProcessSource(&fakeProcSource{err: fmt.Errorf("조회 실패")}),
	)
	c := portpkg.NewChecker(portpkg.WithFinder(finder))

	res, err := c.CheckLocal(80)
	if err != nil {
		t.Fatalf("CheckLocal 오류: %v", err)
	}
	if len(res.Bindings) != 1 {
		t.Fatalf("바인딩 개수 = %d, 기대 1", len(res.Bindings))
	}
	if res.Bindings[0].Name != "unknown" {
		t.Errorf("Name = %q, 기대 unknown", res.Bindings[0].Name)
	}
	if !res.Listening {
		t.Error("PID 0 LISTEN 바인딩도 Listening=true여야 합니다")
	}
}

// TestCheckLocal_ConnectionSourceError는 소켓 조회 실패가 전파되는지 검증합니다.
func TestCheckLocal_ConnectionSourceError(t *testing.T) {
	c := newCheckerWith(nil, fmt.Errorf("네트워크 접근 실패"))

	if _, err := c.CheckLocal(8080); err == nil {
		t.Fatal("연결 소스 오류가 전파되어야 합니다")
	}
}

// ─── CheckLocalRange ─────────────────────────────────────────────────────────

// TestCheckLocalRange_ReturnsEveryPortOrdered는 범위의 모든 포트를 오름차순으로 반환하는지 검증합니다.
func TestCheckLocalRange_ReturnsEveryPortOrdered(t *testing.T) {
	conns := []gonet.ConnectionStat{
		connFull(3001, 5, "127.0.0.1", "LISTEN", uint32(syscall.SOCK_STREAM), uint32(syscall.AF_INET)),
	}
	dialer := &fakeDialer{}

	// CheckLocalRange는 소켓 목록을 1회만 조회해야 합니다.
	spy := &countingConnSource{conns: conns}
	finder := portpkg.NewFinder(
		portpkg.WithConnectionSource(spy),
		portpkg.WithProcessSource(procsFor(conns)),
	)
	c := portpkg.NewChecker(portpkg.WithFinder(finder), portpkg.WithDialer(dialer))

	results, err := c.CheckLocalRange(3000, 3002)
	if err != nil {
		t.Fatalf("CheckLocalRange 오류: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("결과 개수 = %d, 기대 3", len(results))
	}
	for i, want := range []uint16{3000, 3001, 3002} {
		if results[i].Port != want {
			t.Errorf("results[%d].Port = %d, 기대 %d", i, results[i].Port, want)
		}
	}
	if results[0].Listening || len(results[0].Bindings) != 0 {
		t.Errorf("3000은 미사용이어야 합니다: %+v", results[0])
	}
	if !results[1].Listening {
		t.Errorf("3001은 Listening=true여야 합니다: %+v", results[1])
	}
	if results[2].Listening {
		t.Errorf("3002는 미사용이어야 합니다: %+v", results[2])
	}
	if got := spy.callCount(); got != 1 {
		t.Errorf("소켓 목록 조회 횟수 = %d, 기대 1 (범위 조회는 1회만)", got)
	}
}

// countingConnSource는 Connections 호출 횟수를 세는 페이크입니다.
type countingConnSource struct {
	conns []gonet.ConnectionStat
	calls int
}

func (s *countingConnSource) Connections(kind string) ([]gonet.ConnectionStat, error) {
	s.calls++
	return s.conns, nil
}

func (s *countingConnSource) callCount() int { return s.calls }

// ─── CheckRemote ─────────────────────────────────────────────────────────────

// TestCheckRemote_Open는 연결 성공을 open으로 판정하고 연결을 닫는지 검증합니다.
func TestCheckRemote_Open(t *testing.T) {
	conn := &fakeDialConn{}
	dialer := &fakeDialer{outcomes: map[string]dialOutcome{
		"example.com:443": {conn: conn},
	}}
	c := newRemoteChecker(dialer)

	res := c.CheckRemote(context.Background(), "example.com", 443)
	if res.Status != portpkg.ReachOpen {
		t.Fatalf("Status = %q, 기대 %q (detail=%s)", res.Status, portpkg.ReachOpen, res.Detail)
	}
	if res.Host != "example.com" || res.Port != 443 {
		t.Errorf("Host/Port = %s/%d", res.Host, res.Port)
	}
	if res.Latency < 0 {
		t.Errorf("Latency = %v, 기대 >= 0", res.Latency)
	}
	if !conn.closed {
		t.Error("확인 후 연결이 닫혀야 합니다")
	}
	if got := dialer.dialed(); len(got) != 1 || got[0] != "example.com:443" {
		t.Errorf("Dial 주소 = %v, 기대 [example.com:443]", got)
	}
}

// TestCheckRemote_IPv6HostFormat는 IPv6 호스트가 대괄호 주소로 변환되는지 검증합니다.
func TestCheckRemote_IPv6HostFormat(t *testing.T) {
	dialer := &fakeDialer{}
	c := newRemoteChecker(dialer)

	c.CheckRemote(context.Background(), "::1", 80)

	if got := dialer.dialed(); len(got) != 1 || got[0] != "[::1]:80" {
		t.Errorf("Dial 주소 = %v, 기대 [[::1]:80]", got)
	}
}

// TestCheckRemote_ConnectionRefused는 ECONNREFUSED를 closed로 분류하는지 검증합니다.
func TestCheckRemote_ConnectionRefused(t *testing.T) {
	dialer := &fakeDialer{outcomes: map[string]dialOutcome{
		"10.0.0.1:8080": {err: fmt.Errorf("dial tcp: %w", syscall.ECONNREFUSED)},
	}}
	c := newRemoteChecker(dialer)

	res := c.CheckRemote(context.Background(), "10.0.0.1", 8080)
	if res.Status != portpkg.ReachClosed {
		t.Fatalf("Status = %q, 기대 %q", res.Status, portpkg.ReachClosed)
	}
	if res.Detail == "" {
		t.Error("closed 상태에는 원인 설명이 있어야 합니다")
	}
}

// TestCheckRemote_DeadlineExceededIsFiltered는 응답 없음(타임아웃)을 filtered로 분류하는지 검증합니다.
func TestCheckRemote_DeadlineExceededIsFiltered(t *testing.T) {
	dialer := &fakeDialer{outcomes: map[string]dialOutcome{
		"10.0.0.2:22": {err: fmt.Errorf("dial tcp: %w", context.DeadlineExceeded)},
	}}
	c := newRemoteChecker(dialer)

	if res := c.CheckRemote(context.Background(), "10.0.0.2", 22); res.Status != portpkg.ReachFiltered {
		t.Fatalf("Status = %q, 기대 %q", res.Status, portpkg.ReachFiltered)
	}
}

// TestCheckRemote_NetTimeoutIsFiltered는 net.Error Timeout을 filtered로 분류하는지 검증합니다.
func TestCheckRemote_NetTimeoutIsFiltered(t *testing.T) {
	dialer := &fakeDialer{outcomes: map[string]dialOutcome{
		"10.0.0.3:22": {err: fakeTimeoutErr{}},
	}}
	c := newRemoteChecker(dialer)

	if res := c.CheckRemote(context.Background(), "10.0.0.3", 22); res.Status != portpkg.ReachFiltered {
		t.Fatalf("Status = %q, 기대 %q", res.Status, portpkg.ReachFiltered)
	}
}

// TestCheckRemote_HostUnreachable는 라우팅 실패를 unreachable로 분류하는지 검증합니다.
func TestCheckRemote_HostUnreachable(t *testing.T) {
	dialer := &fakeDialer{outcomes: map[string]dialOutcome{
		"10.0.0.4:22": {err: fmt.Errorf("dial tcp: %w", syscall.EHOSTUNREACH)},
	}}
	c := newRemoteChecker(dialer)

	if res := c.CheckRemote(context.Background(), "10.0.0.4", 22); res.Status != portpkg.ReachUnreachable {
		t.Fatalf("Status = %q, 기대 %q", res.Status, portpkg.ReachUnreachable)
	}
}

// TestCheckRemote_DNSError는 호스트 이름 해석 실패를 error로 분류하는지 검증합니다.
func TestCheckRemote_DNSError(t *testing.T) {
	dialer := &fakeDialer{outcomes: map[string]dialOutcome{
		"nonexistent.invalid:80": {err: &net.DNSError{Err: "no such host", Name: "nonexistent.invalid", IsNotFound: true}},
	}}
	c := newRemoteChecker(dialer)

	res := c.CheckRemote(context.Background(), "nonexistent.invalid", 80)
	if res.Status != portpkg.ReachError {
		t.Fatalf("Status = %q, 기대 %q", res.Status, portpkg.ReachError)
	}
	if !strings.Contains(res.Detail, "nonexistent.invalid") {
		t.Errorf("Detail = %q, 호스트 이름을 포함해야 합니다", res.Detail)
	}
}

// TestCheckRemote_UnknownError는 분류 불가 오류를 error로 전달하는지 검증합니다.
func TestCheckRemote_UnknownError(t *testing.T) {
	dialer := &fakeDialer{outcomes: map[string]dialOutcome{
		"10.0.0.5:22": {err: fmt.Errorf("이상한 오류")},
	}}
	c := newRemoteChecker(dialer)

	res := c.CheckRemote(context.Background(), "10.0.0.5", 22)
	if res.Status != portpkg.ReachError {
		t.Fatalf("Status = %q, 기대 %q", res.Status, portpkg.ReachError)
	}
	if res.Detail != "이상한 오류" {
		t.Errorf("Detail = %q, 기대 \"이상한 오류\"", res.Detail)
	}
}

// TestCheckRemote_EmptyHost는 호스트가 비면 연결 시도 없이 error를 반환하는지 검증합니다.
func TestCheckRemote_EmptyHost(t *testing.T) {
	dialer := &fakeDialer{}
	c := newRemoteChecker(dialer)

	res := c.CheckRemote(context.Background(), "   ", 80)
	if res.Status != portpkg.ReachError {
		t.Fatalf("Status = %q, 기대 %q", res.Status, portpkg.ReachError)
	}
	if got := dialer.callCount(); got != 0 {
		t.Errorf("Dial 호출 횟수 = %d, 기대 0", got)
	}
}

// TestCheckRemote_AppliesConfiguredTimeout는 설정한 타임아웃이 Dial 컨텍스트에 반영되는지 검증합니다.
func TestCheckRemote_AppliesConfiguredTimeout(t *testing.T) {
	dialer := &fakeDialer{}
	c := newRemoteChecker(dialer, portpkg.WithCheckTimeout(50*time.Millisecond))

	c.CheckRemote(context.Background(), "10.0.0.6", 22)

	deadline, ok := dialer.lastDeadline()
	if !ok {
		t.Fatal("Dial 컨텍스트에 마감 시각이 설정되어야 합니다")
	}
	remaining := time.Until(deadline)
	if remaining <= 0 || remaining > 60*time.Millisecond {
		t.Errorf("남은 타임아웃 = %v, 기대 (0, 60ms]", remaining)
	}
}

// TestCheckRemote_DefaultTimeout는 기본 타임아웃이 적용되는지 검증합니다.
func TestCheckRemote_DefaultTimeout(t *testing.T) {
	dialer := &fakeDialer{}
	c := newRemoteChecker(dialer)

	c.CheckRemote(context.Background(), "10.0.0.7", 22)

	deadline, ok := dialer.lastDeadline()
	if !ok {
		t.Fatal("Dial 컨텍스트에 마감 시각이 설정되어야 합니다")
	}
	remaining := time.Until(deadline)
	if remaining <= 0 || remaining > portpkg.DefaultCheckTimeout {
		t.Errorf("남은 타임아웃 = %v, 기대 (0, %v]", remaining, portpkg.DefaultCheckTimeout)
	}
}

// TestCheckRemote_PackageFunction은 패키지 수준 편의 함수가 동작하는지 검증합니다.
// (실제 Dialer를 사용하므로 로컬에 바인딩된 포트가 없으면 closed/error가 정상입니다.)
func TestCheckRemote_PackageFunction(t *testing.T) {
	res := portpkg.CheckRemote(context.Background(), "127.0.0.1", 1)
	if res == nil {
		t.Fatal("CheckRemote는 nil이 아닌 결과를 반환해야 합니다")
	}
	if res.Host != "127.0.0.1" || res.Port != 1 {
		t.Errorf("결과 대상 = %s:%d", res.Host, res.Port)
	}
	if res.Status == "" {
		t.Error("Status가 비어 있습니다")
	}
}
