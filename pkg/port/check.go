// 이 파일은 로컬 포트 바인딩 확인과 원격 TCP 도달성 확인 기능을 제공합니다.
// 소켓 목록 조회는 Finder(ConnectionSource)로, TCP 연결은 Dialer 인터페이스로 격리하여
// 단위 테스트에서 페이크 주입만으로 결정적 검증이 가능합니다(실제 네트워크 불필요).
package port

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// DefaultCheckTimeout은 원격 도달성 확인의 기본 타임아웃입니다.
const DefaultCheckTimeout = 3 * time.Second

// Binding은 로컬 소켓 바인딩 1건의 상세 정보입니다.
type Binding struct {
	Port  uint16
	IP    string
	Proto string // tcp, tcp6, udp, udp6
	State string // LISTEN, ESTABLISHED 등. UDP는 빈 문자열인 경우가 많습니다.
	PID   int32
	Name  string
}

// Address는 "IP:PORT" 형식의 주소 문자열을 반환합니다 (IPv6는 대괄호로 감쌉니다).
func (b Binding) Address() string {
	return net.JoinHostPort(b.IP, strconv.Itoa(int(b.Port)))
}

// IsListening은 이 바인딩이 수신 대기 상태인지 반환합니다.
// TCP는 LISTEN 상태일 때만 수신 대기이며, UDP는 바인딩 자체가 수신 대기입니다.
func (b Binding) IsListening() bool {
	if strings.HasPrefix(b.Proto, "udp") {
		return true
	}
	return strings.EqualFold(b.State, "LISTEN")
}

// LocalPortResult는 로컬 포트 1개에 대한 확인 결과입니다.
type LocalPortResult struct {
	Port      uint16
	Listening bool
	Bindings  []Binding // 바인딩이 없으면 비어 있습니다.
}

// ReachStatus는 원격 TCP 도달성 상태입니다.
type ReachStatus string

const (
	// ReachOpen은 TCP 연결(핸드셰이크)이 성공한 상태입니다.
	ReachOpen ReachStatus = "open"
	// ReachClosed는 연결이 거부된 상태입니다 (호스트는 응답하지만 포트가 닫힘).
	ReachClosed ReachStatus = "closed"
	// ReachFiltered는 응답이 없는 상태입니다 (방화벽 DROP 또는 호스트 다운 가능성).
	ReachFiltered ReachStatus = "filtered"
	// ReachUnreachable은 라우팅 실패로 호스트/네트워크에 도달할 수 없는 상태입니다.
	ReachUnreachable ReachStatus = "unreachable"
	// ReachError는 그 밖의 오류(이름 해석 실패 등)로 판정할 수 없는 상태입니다.
	ReachError ReachStatus = "error"
)

// Open은 도달성 확인이 성공(open)했는지 반환합니다.
func (s ReachStatus) Open() bool { return s == ReachOpen }

// ReachResult는 host:port 원격 도달성 확인 결과입니다.
// 도달 실패는 오류가 아니라 Status/Detail로 표현됩니다 (closed/filtered 등).
type ReachResult struct {
	Host    string
	Port    uint16
	Status  ReachStatus
	Latency time.Duration
	Detail  string // 원인 설명 (오류 메시지 등)
}

// Dialer는 TCP 연결을 생성하는 추상화입니다.
// 기본 구현은 *net.Dialer이며, 테스트에서는 페이크를 주입해 실제 네트워크 없이 검증합니다.
type Dialer interface {
	DialContext(ctx context.Context, network, address string) (net.Conn, error)
}

// CheckOption은 Checker 구성을 변경하는 함수 옵션입니다.
type CheckOption func(*Checker)

// WithFinder는 로컬 확인에 사용할 Finder를 교체합니다. (테스트용)
func WithFinder(f *Finder) CheckOption {
	return func(c *Checker) { c.finder = f }
}

// WithDialer는 원격 확인에 사용할 Dialer를 교체합니다. (테스트용)
func WithDialer(d Dialer) CheckOption {
	return func(c *Checker) { c.dialer = d }
}

// WithCheckTimeout은 원격 확인 타임아웃을 설정합니다 (0 이하는 무시).
func WithCheckTimeout(d time.Duration) CheckOption {
	return func(c *Checker) {
		if d > 0 {
			c.timeout = d
		}
	}
}

// Checker는 로컬 포트 바인딩 확인과 원격 TCP 도달성 확인을 담당합니다.
// 의존성(Finder, Dialer, 타임아웃)을 주입할 수 있어 결정적 단위 테스트를 지원합니다.
type Checker struct {
	finder  *Finder
	dialer  Dialer
	timeout time.Duration
}

// NewChecker는 실제 시스템 소스와 기본 타임아웃을 사용하는 Checker를 생성합니다.
func NewChecker(opts ...CheckOption) *Checker {
	c := &Checker{
		finder:  NewFinder(),
		dialer:  &net.Dialer{},
		timeout: DefaultCheckTimeout,
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// CheckLocal은 단일 포트의 로컬 바인딩 상태를 확인합니다.
func (c *Checker) CheckLocal(port uint16) (*LocalPortResult, error) {
	results, err := c.CheckLocalRange(port, port)
	if err != nil {
		return nil, err
	}
	return results[0], nil
}

// CheckLocalRange는 start~end 범위의 로컬 바인딩 상태를 포트 오름차순으로 반환합니다.
// 소켓 목록은 1회만 조회하며, 바인딩이 없는 포트도 Listening=false 결과로 포함합니다.
func (c *Checker) CheckLocalRange(start, end uint16) ([]*LocalPortResult, error) {
	bindings, err := c.finder.BindingsByPortRange(start, end)
	if err != nil {
		return nil, err
	}

	byPort := make(map[uint16][]Binding, len(bindings))
	for _, b := range bindings {
		byPort[b.Port] = append(byPort[b.Port], b)
	}

	results := make([]*LocalPortResult, 0, int(end)-int(start)+1)
	for p := uint32(start); p <= uint32(end); p++ {
		res := &LocalPortResult{Port: uint16(p), Bindings: byPort[uint16(p)]}
		for _, b := range res.Bindings {
			if b.IsListening() {
				res.Listening = true
				break
			}
		}
		results = append(results, res)
	}
	return results, nil
}

// CheckRemote는 host:port로 TCP 연결을 시도하여 도달성을 판정합니다.
// 연결 시도는 타임아웃 내에서만 수행하며, 성공한 연결은 즉시 닫습니다(확인 목적).
func (c *Checker) CheckRemote(ctx context.Context, host string, port uint16) *ReachResult {
	res := &ReachResult{Host: host, Port: port}
	if strings.TrimSpace(host) == "" {
		res.Status = ReachError
		res.Detail = "호스트가 지정되지 않았습니다"
		return res
	}
	if ctx == nil {
		ctx = context.Background()
	}

	dialCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	start := time.Now()
	conn, err := c.dialer.DialContext(dialCtx, "tcp", net.JoinHostPort(host, strconv.Itoa(int(port))))
	res.Latency = time.Since(start)
	if err != nil {
		res.Status, res.Detail = classifyDialError(err)
		return res
	}

	res.Status = ReachOpen
	if cerr := conn.Close(); cerr != nil {
		res.Detail = fmt.Sprintf("연결은 성공했지만 닫기에 실패했습니다: %v", cerr)
	}
	return res
}

// classifyDialError는 연결 실패 원인을 도달성 상태와 설명으로 분류합니다.
func classifyDialError(err error) (ReachStatus, string) {
	switch {
	case errors.Is(err, syscall.ECONNREFUSED):
		return ReachClosed, "연결이 거부되었습니다 (호스트는 응답하지만 해당 포트가 닫혀 있음)"
	case errors.Is(err, syscall.EHOSTUNREACH), errors.Is(err, syscall.ENETUNREACH):
		return ReachUnreachable, "호스트 또는 네트워크에 도달할 수 없습니다 (라우팅/게이트웨이 확인)"
	case errors.Is(err, context.DeadlineExceeded):
		return ReachFiltered, "응답이 없습니다 (방화벽 DROP 또는 호스트 다운 가능성)"
	case errors.Is(err, context.Canceled):
		return ReachError, "확인이 취소되었습니다"
	}

	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return ReachError, fmt.Sprintf("호스트 이름을 확인할 수 없습니다: %s", dnsErr.Name)
	}

	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return ReachFiltered, "응답이 없습니다 (방화벽 DROP 또는 호스트 다운 가능성)"
	}
	return ReachError, err.Error()
}

// 아래는 패키지 수준의 편의 함수들입니다 (기본 설정의 Checker 사용).

var defaultChecker = NewChecker()

// CheckLocal은 특정 포트의 로컬 바인딩 상태를 확인합니다.
func CheckLocal(port uint16) (*LocalPortResult, error) {
	return defaultChecker.CheckLocal(port)
}

// CheckLocalRange는 start~end 범위의 로컬 바인딩 상태를 반환합니다.
func CheckLocalRange(start, end uint16) ([]*LocalPortResult, error) {
	return defaultChecker.CheckLocalRange(start, end)
}

// CheckRemote는 host:port의 TCP 도달성을 확인합니다.
func CheckRemote(ctx context.Context, host string, port uint16) *ReachResult {
	return defaultChecker.CheckRemote(ctx, host, port)
}
