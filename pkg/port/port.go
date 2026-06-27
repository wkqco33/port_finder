package port

import (
	"fmt"
	"sort"
	"time"

	"github.com/shirou/gopsutil/v4/net"
	"github.com/shirou/gopsutil/v4/process"
)

// ProcessInfo는 포트를 사용 중인 프로세스의 핵심 데이터 구조체입니다.
type ProcessInfo struct {
	PID  int32
	Name string
	Port uint16
}

func getProcessName(pid int32) string {
	p, err := process.NewProcess(pid)
	if err != nil {
		return "unknown"
	}
	name, err := p.Name()
	if err != nil {
		return "unknown"
	}
	return name
}

type portPid struct {
	port uint16
	pid  int32
}

// FindByPort는 특정 포트를 사용하는 모든 프로세스를 검색하여 반환합니다.
// 찾지 못한 경우 빈 슬라이스와 nil을 반환합니다.
func FindByPort(targetPort uint16) ([]*ProcessInfo, error) {
	return FindByPortRange(targetPort, targetPort)
}

// FindByPortRange는 start~end 범위 안에서 사용 중인 포트의 프로세스를 모두 반환합니다.
func FindByPortRange(start, end uint16) ([]*ProcessInfo, error) {
	conns, err := net.Connections("inet")
	if err != nil {
		return nil, fmt.Errorf("네트워크 정보를 가져올 수 없습니다: %w", err)
	}

	seen := make(map[portPid]bool)
	var results []*ProcessInfo

	nameCache := make(map[int32]string)
	getProcName := func(pid int32) string {
		if name, ok := nameCache[pid]; ok {
			return name
		}
		name := getProcessName(pid)
		nameCache[pid] = name
		return name
	}

	for i := range conns {
		p := uint16(conns[i].Laddr.Port)
		if p < start || p > end || conns[i].Pid <= 0 {
			continue
		}
		key := portPid{port: p, pid: conns[i].Pid}
		if seen[key] {
			continue
		}
		seen[key] = true
		results = append(results, &ProcessInfo{
			PID:  conns[i].Pid,
			Name: getProcName(conns[i].Pid),
			Port: p,
		})
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Port < results[j].Port
	})
	return results, nil
}

// ListAll은 현재 사용 중인 모든 포트 목록을 반환합니다.
func ListAll() ([]*ProcessInfo, error) {
	return FindByPortRange(1, 65535)
}

// KillProcessByPID는 특정 PID를 가진 프로세스를 즉시 종료합니다.
func KillProcessByPID(pid int32) error {
	p, err := process.NewProcess(pid)
	if err != nil {
		return fmt.Errorf("프로세스를 찾을 수 없습니다: %w", err)
	}
	return p.Kill()
}

// KillProcessGracefully는 SIGTERM을 먼저 보내고, timeout 내에 종료되지 않으면 SIGKILL로 강제 종료합니다.
func KillProcessGracefully(pid int32, timeout time.Duration) error {
	p, err := process.NewProcess(pid)
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
