// Package port는 시스템 네트워크 포트 점유 현황 조회 및 프로세스 제어(종료) 기능을 제공합니다.
// OS 연동 라이브러리(gopsutil)를 추상화 인터페이스 뒤에 격리하여 단위 테스트에서 페이크 주입을 지원합니다.
package port

import (
	"fmt"
	"sort"
	"syscall"
	"time"

	"github.com/shirou/gopsutil/v4/net"
	"github.com/shirou/gopsutil/v4/process"
)

// unknownProcessName은 프로세스 이름을 조회할 수 없을 때 사용하는 대체 이름입니다.
const unknownProcessName = "unknown"

// ProcessInfo는 포트를 사용 중인 프로세스의 핵심 데이터 구조체입니다.
type ProcessInfo struct {
	PID  int32
	Name string
	Port uint16
}

// Process는 이 패키지가 제어할 수 있는 최소한의 프로세스 표면입니다.
// 실제 구현(*process.Process)뿐 아니라 테스트용 페이크도 이 인터페이스로 대체할 수 있습니다.
type Process interface {
	Name() (string, error)
	Kill() error
	Terminate() error
	IsRunning() (bool, error)
}

// ConnectionSource는 네트워크 연결 목록을 가져오는 소스입니다.
// gopsutil의 net.Connections(전역 함수)를 추상화하여 단위 테스트에서 페이크로 대체할 수 있게 합니다.
type ConnectionSource interface {
	Connections(kind string) ([]net.ConnectionStat, error)
}

// ProcessSource는 PID로 프로세스 객체를 생성하는 팩토리입니다.
type ProcessSource interface {
	NewProcess(pid int32) (Process, error)
}

// realConnSource는 ConnectionSource의 기본 구현입니다.
type realConnSource struct{}

func (realConnSource) Connections(kind string) ([]net.ConnectionStat, error) {
	return net.Connections(kind)
}

// realProcSource는 ProcessSource의 기본 구현입니다.
type realProcSource struct{}

func (realProcSource) NewProcess(pid int32) (Process, error) {
	return process.NewProcess(pid)
}

// Option은 Finder 구성을 변경하는 함수 옵션입니다.
type Option func(*Finder)

// WithConnectionSource는 네트워크 연결 소스를 교체합니다. (테스트용)
func WithConnectionSource(src ConnectionSource) Option {
	return func(f *Finder) { f.conns = src }
}

// WithProcessSource는 프로세스 소스를 교체합니다. (테스트용)
func WithProcessSource(src ProcessSource) Option {
	return func(f *Finder) { f.procs = src }
}

// Finder는 포트 검색과 프로세스 종료 로직을 담당합니다.
// 의존성(연결 소스, 프로세스 소스)을 주입 가능하여 결정적 단위 테스트를 지원합니다.
type Finder struct {
	conns ConnectionSource
	procs ProcessSource
}

// NewFinder는 실제 시스템 소스를 사용하는 Finder를 생성합니다.
// 옵션으로 테스트용 소스를 주입할 수 있습니다.
func NewFinder(opts ...Option) *Finder {
	f := &Finder{
		conns: realConnSource{},
		procs: realProcSource{},
	}
	for _, o := range opts {
		o(f)
	}
	return f
}

// getProcessName은 PID에 해당하는 프로세스 이름을 반환하며, 실패 시 "unknown"을 반환합니다.
func (f *Finder) getProcessName(pid int32) string {
	p, err := f.procs.NewProcess(pid)
	if err != nil {
		return unknownProcessName
	}
	name, err := p.Name()
	if err != nil {
		return unknownProcessName
	}
	return name
}

// protoName은 gopsutil 소켓 타입/패밀리를 프로토콜 이름으로 변환합니다.
// 소켓 타입을 알 수 없는 플랫폼에서는 TCP로 간주합니다.
func protoName(sockType, family uint32) string {
	v6 := family == uint32(syscall.AF_INET6)
	switch sockType {
	case uint32(syscall.SOCK_DGRAM):
		if v6 {
			return "udp6"
		}
		return "udp"
	default:
		if v6 {
			return "tcp6"
		}
		return "tcp"
	}
}

// BindingsByPortRange는 start~end 범위의 로컬 소켓 바인딩 내역을 상세 정보와 함께 반환합니다.
// PID가 없는 커널 소켓도 바인딩 사실 자체는 유효하므로 포함하며(이름은 unknown),
// 결과는 포트 → 프로토콜 → IP 오름차순으로 정렬합니다.
func (f *Finder) BindingsByPortRange(start, end uint16) ([]Binding, error) {
	conns, err := f.conns.Connections("inet")
	if err != nil {
		return nil, fmt.Errorf("네트워크 정보를 가져올 수 없습니다: %w", err)
	}

	nameCache := make(map[int32]string)
	getProcName := func(pid int32) string {
		// PID 0(커널 소켓)은 프로세스 조회 대상이 아닙니다.
		if pid <= 0 {
			return unknownProcessName
		}
		if name, ok := nameCache[pid]; ok {
			return name
		}
		name := f.getProcessName(pid)
		nameCache[pid] = name
		return name
	}

	var bindings []Binding
	for i := range conns {
		p := uint16(conns[i].Laddr.Port)
		if p < start || p > end {
			continue
		}
		bindings = append(bindings, Binding{
			Port:  p,
			IP:    conns[i].Laddr.IP,
			Proto: protoName(conns[i].Type, conns[i].Family),
			State: conns[i].Status,
			PID:   conns[i].Pid,
			Name:  getProcName(conns[i].Pid),
		})
	}

	sort.SliceStable(bindings, func(i, j int) bool {
		if bindings[i].Port != bindings[j].Port {
			return bindings[i].Port < bindings[j].Port
		}
		if bindings[i].Proto != bindings[j].Proto {
			return bindings[i].Proto < bindings[j].Proto
		}
		return bindings[i].IP < bindings[j].IP
	})
	return bindings, nil
}

type portPid struct {
	port uint16
	pid  int32
}

// FindByPort는 특정 포트를 사용하는 모든 프로세스를 검색하여 반환합니다.
// 찾지 못한 경우 빈 슬라이스와 nil을 반환합니다.
func (f *Finder) FindByPort(targetPort uint16) ([]*ProcessInfo, error) {
	return f.FindByPortRange(targetPort, targetPort)
}

// FindByPortRange는 start~end 범위 안에서 사용 중인 포트의 프로세스를 모두 반환합니다.
// 바인딩 상세 정보는 BindingsByPortRange에서 가져오고, (포트, PID) 기준으로 중복을 제거합니다.
func (f *Finder) FindByPortRange(start, end uint16) ([]*ProcessInfo, error) {
	bindings, err := f.BindingsByPortRange(start, end)
	if err != nil {
		return nil, err
	}

	seen := make(map[portPid]bool)
	var results []*ProcessInfo
	for _, b := range bindings {
		// PID가 없는 커널 소켓은 프로세스 정보가 없으므로 제외합니다.
		if b.PID <= 0 {
			continue
		}
		key := portPid{port: b.Port, pid: b.PID}
		if seen[key] {
			continue
		}
		seen[key] = true
		results = append(results, &ProcessInfo{
			PID:  b.PID,
			Name: b.Name,
			Port: b.Port,
		})
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Port < results[j].Port
	})
	return results, nil
}

// ListAll은 현재 사용 중인 모든 포트 목록을 반환합니다.
func (f *Finder) ListAll() ([]*ProcessInfo, error) {
	return f.FindByPortRange(1, 65535)
}

// KillProcessByPID는 특정 PID를 가진 프로세스를 즉시 종료합니다.
func (f *Finder) KillProcessByPID(pid int32) error {
	p, err := f.procs.NewProcess(pid)
	if err != nil {
		return fmt.Errorf("프로세스를 찾을 수 없습니다: %w", err)
	}
	return p.Kill()
}

// KillProcessGracefully는 SIGTERM을 먼저 보내고, timeout 내에 종료되지 않으면 SIGKILL로 강제 종료합니다.
func (f *Finder) KillProcessGracefully(pid int32, timeout time.Duration) error {
	p, err := f.procs.NewProcess(pid)
	if err != nil {
		return fmt.Errorf("프로세스를 찾을 수 없습니다: %w", err)
	}

	if err := p.Terminate(); err != nil {
		return p.Kill()
	}

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if running, err := p.IsRunning(); err != nil || !running {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}

	return p.Kill()
}

// 아래는 패키지 수준의 편의 함수들입니다.
// 내부적으로 기본 설정의 Finder를 사용하므로 기존 호출 코드가 그대로 동작합니다.

var defaultFinder = NewFinder()

// FindByPort는 특정 포트를 사용하는 모든 프로세스를 검색하여 반환합니다.
func FindByPort(targetPort uint16) ([]*ProcessInfo, error) {
	return defaultFinder.FindByPort(targetPort)
}

// FindByPortRange는 start~end 범위 안에서 사용 중인 포트의 프로세스를 모두 반환합니다.
func FindByPortRange(start, end uint16) ([]*ProcessInfo, error) {
	return defaultFinder.FindByPortRange(start, end)
}

// ListAll은 현재 사용 중인 모든 포트 목록을 반환합니다.
func ListAll() ([]*ProcessInfo, error) {
	return defaultFinder.ListAll()
}

// KillProcessByPID는 특정 PID를 가진 프로세스를 즉시 종료합니다.
func KillProcessByPID(pid int32) error {
	return defaultFinder.KillProcessByPID(pid)
}

// KillProcessGracefully는 SIGTERM을 먼저 보내고, timeout 내에 종료되지 않으면 SIGKILL로 강제 종료합니다.
func KillProcessGracefully(pid int32, timeout time.Duration) error {
	return defaultFinder.KillProcessGracefully(pid, timeout)
}
