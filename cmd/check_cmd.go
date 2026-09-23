// check_cmd.go는 `poff check` 서브커맨드(로컬 포트 상태 / 원격 TCP 도달성 확인)를 구현합니다.
// 확인 로직은 pkg/port의 Checker에 위임하고, 이 파일은 입출력과 흐름만 담당합니다.
package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	portpkg "port_finder/pkg/port"

	"github.com/fatih/color"
	"github.com/wkqco33/wcli"
)

// checkConcurrency는 원격 범위 확인 시 동시에 시도할 최대 연결 수입니다.
// 전체 스캔 시간을 제한하면서 대상 호스트에 과도한 부하를 주지 않도록 고정합니다.
const checkConcurrency = 16

// checkOps는 check 서브커맨드가 사용하는 port 패키지의 표면입니다.
// 테스트에서 페이크로 대체할 수 있어 CLI 흐름을 격리해서 검증할 수 있습니다.
type checkOps interface {
	CheckLocalRange(start, end uint16) ([]*portpkg.LocalPortResult, error)
	CheckRemote(ctx context.Context, host string, port uint16) *portpkg.ReachResult
}

// checkApp은 check 서브커맨드가 갖는 외부 의존성(스트림 + 확인 ops + 플래그)입니다.
type checkApp struct {
	Out  io.Writer
	ErrW io.Writer

	Ops checkOps

	// PortStr은 확인할 포트 또는 범위입니다 (예: "8080", "3000-3010").
	PortStr string
	// Host는 원격 확인 대상입니다. 비어 있으면 로컬 바인딩 상태를 확인합니다.
	Host string
	// Timeout은 원격 연결 타임아웃입니다.
	Timeout time.Duration

	JSON  bool
	Quiet bool
}

// newCheckCommand는 `poff check` 서브커맨드를 생성합니다.
func newCheckCommand() *wcli.Command {
	var (
		portStr string
		host    string
		timeout time.Duration
		jsonOut bool
		quiet   bool
		noColor bool
	)

	checkCmd := &wcli.Command{
		Use:   "check",
		Short: "포트 상태 확인 (로컬 바인딩 / 원격 TCP 도달성)",
		Long: `지정한 포트가 사용 중인지(로컬) 또는 원격 호스트와 통신이 되는지(원격) 확인합니다.

로컬 확인 (기본):
  현재 PC에서 해당 포트를 바인딩한 소켓과 프로세스를 조회합니다.
  TCP는 LISTEN 상태, UDP는 바인딩 여부로 수신 대기를 판정합니다.
  poff check -p 8080
  poff check -p 8000-8010

원격 확인 (-H/--host 지정):
  해당 호스트:포트로 TCP 연결(핸드셰이크)을 시도해 도달성을 판정합니다.
  ICMP(ping)와 달리 포트 단위 확인이 가능하며, 방화벽이 ICMP를 막아도 판정할 수 있습니다.
  결과: open(연결 성공) / closed(연결 거부) / filtered(응답 없음) /
        unreachable(라우팅 실패) / error(이름 해석 실패 등)
  poff check -H 192.168.0.10 -p 8080
  poff check -H db.internal -p 5432 -t 5s

출력 규칙 (clig.dev):
  결과 데이터는 stdout, 진행/요약 메시지는 stderr로 출력합니다.
  --json은 stdout에 유효한 JSON 배열만 출력하며, 대상이 없으면 []를 출력합니다.

종료 코드:
  0: 확인 대상이 수신 대기(LISTEN/UDP 바인딩)이거나 원격 연결이 성공(open)한 경우
  1: 그 외 (미사용, 연결 거부, 응답 없음, 도달 불가, 오류)
  2: 인자/플래그 사용 오류`,
		Run: func(ctx *wcli.Context) error {
			if noColor {
				color.NoColor = true
			}
			return newRealCheckApp(portStr, host, timeout, jsonOut, quiet).Run()
		},
	}

	checkCmd.Flags().StringVar(&portStr, "port", "p", "", "확인할 포트 번호 또는 범위 (예: 8080, 3000-3010)")
	checkCmd.Flags().StringVar(&host, "host", "H", "", "원격 호스트(IP/도메인). 미지정 시 로컬 포트 상태를 확인")
	checkCmd.Flags().DurationVar(&timeout, "timeout", "t", portpkg.DefaultCheckTimeout, "원격 연결 타임아웃 (예: 500ms, 5s)")
	checkCmd.Flags().BoolVar(&jsonOut, "json", "j", false, "JSON 형식으로 출력")
	checkCmd.Flags().BoolVar(&quiet, "quiet", "q", false, "진행 및 요약 메시지 억제")
	checkCmd.Flags().BoolVar(&noColor, "no-color", "", false, "컬러 출력을 비활성화")

	return checkCmd
}

// newRealCheckApp은 실제 스트림과 실제 Checker를 사용하는 checkApp을 구성합니다.
func newRealCheckApp(portStr, host string, timeout time.Duration, jsonOut, quiet bool) *checkApp {
	effectiveTimeout := timeoutOrDefault(timeout)
	return &checkApp{
		Out:     os.Stdout,
		ErrW:    os.Stderr,
		Ops:     portpkg.NewChecker(portpkg.WithCheckTimeout(effectiveTimeout)),
		PortStr: portStr,
		Host:    host,
		Timeout: effectiveTimeout,
		JSON:    jsonOut,
		Quiet:   quiet,
	}
}

// timeoutOrDefault는 0 이하의 타임아웃을 기본값으로 대체합니다.
func timeoutOrDefault(d time.Duration) time.Duration {
	if d <= 0 {
		return portpkg.DefaultCheckTimeout
	}
	return d
}

// logProgress는 진행/요약 메시지를 stderr로 출력합니다 (JSON/quiet 모드에서는 억제).
func (a *checkApp) logProgress(format string, args ...any) {
	if a.JSON || a.Quiet {
		return
	}
	fmt.Fprintf(a.ErrW, format, args...)
}

// Run은 포트 상태 확인을 실행합니다.
// 열린(수신 대기 또는 도달 가능) 포트가 하나도 없으면 메시지 없이 종료 코드 1을 반환합니다.
func (a *checkApp) Run() error {
	if strings.TrimSpace(a.PortStr) == "" {
		return usageError("확인할 포트가 필요합니다 (사용: poff check -p <PORT> [-H <HOST>])")
	}
	start, end, err := ParsePortArg(a.PortStr)
	if err != nil {
		return usageError("%w", err)
	}

	if a.Host != "" {
		return a.runRemote(start, end)
	}
	return a.runLocal(start, end)
}

// ─── 로컬 확인 ───────────────────────────────────────────────────────────────

// runLocal은 로컬 바인딩 상태를 확인해 결과를 출력합니다.
func (a *checkApp) runLocal(start, end uint16) error {
	if start == end {
		a.logProgress("%s 포트 %d %s\n",
			headerStyle("🔍"), start, warnStyle("로컬 사용 여부를 확인 중입니다..."))
	} else {
		a.logProgress("%s 포트 %s%d-%d%s %s\n",
			headerStyle("🔍"), warnStyle("["), start, end, warnStyle("]"),
			warnStyle("로컬 사용 여부를 확인 중입니다..."))
	}

	results, err := a.Ops.CheckLocalRange(start, end)
	if err != nil {
		return err
	}

	if a.JSON {
		if err := a.printLocalJSON(results); err != nil {
			return err
		}
	} else if start == end {
		a.printLocalSingle(results[0])
	} else {
		a.printLocalRange(start, end, results)
	}

	if anyListening(results) {
		return nil
	}
	return &silentExitError{code: ExitCodeError}
}

// anyListening은 수신 대기 중인 포트가 하나라도 있는지 반환합니다.
func anyListening(results []*portpkg.LocalPortResult) bool {
	for _, r := range results {
		if r.Listening {
			return true
		}
	}
	return false
}

// printLocalSingle은 단일 포트의 상세 결과를 출력합니다.
func (a *checkApp) printLocalSingle(res *portpkg.LocalPortResult) {
	switch {
	case res.Listening:
		fmt.Fprintf(a.Out, "%s 포트 %d: %s\n", successStyle("✅"), res.Port, headerStyle("LISTEN 중 (수신 대기)"))
	case len(res.Bindings) > 0:
		fmt.Fprintf(a.Out, "%s 포트 %d: %s\n", warnStyle("⚠️"), res.Port, warnStyle("LISTEN 아님 (사용 중이지만 수신 대기가 아님)"))
	default:
		fmt.Fprintf(a.Out, "%s 포트 %d: %s\n", warnStyle("⚠️"), res.Port, valueStyle("사용 중이 아님 (바인딩된 소켓 없음)"))
		return
	}

	for _, b := range res.Bindings {
		fmt.Fprintf(a.Out, "   %s %-10s : %s\n", keyStyle("•"), "PROTO", valueStyle(b.Proto))
		fmt.Fprintf(a.Out, "   %s %-10s : %s\n", keyStyle("•"), "ADDRESS", valueStyle(b.Address()))
		fmt.Fprintf(a.Out, "   %s %-10s : %s\n", keyStyle("•"), "STATE", valueStyle(stateOrDash(b.State)))
		fmt.Fprintf(a.Out, "   %s %-10s : %s\n", keyStyle("•"), "PID", valueStyle(strconv.Itoa(int(b.PID))))
		fmt.Fprintf(a.Out, "   %s %-10s : %s\n", keyStyle("•"), "NAME", valueStyle(b.Name))
		fmt.Fprintln(a.Out)
	}
}

// printLocalRange는 범위 확인 결과를 표로 출력합니다 (사용 중인 포트만 표시).
func (a *checkApp) printLocalRange(start, end uint16, results []*portpkg.LocalPortResult) {
	occupied := make([]*portpkg.LocalPortResult, 0, len(results))
	for _, r := range results {
		if len(r.Bindings) > 0 {
			occupied = append(occupied, r)
		}
	}

	a.logProgress("%s 확인한 %d개 포트 중 사용 중: %d개\n", dimStyle("ℹ️"), len(results), len(occupied))

	if len(occupied) == 0 {
		fmt.Fprintf(a.Out, "%s 포트 %d-%d 범위에서 사용 중인 포트가 없습니다.\n", warnStyle("⚠️"), start, end)
		return
	}

	fmt.Fprintf(a.Out, "%s %s\n\n", headerStyle("📋"), headerStyle(fmt.Sprintf("로컬 포트 %d-%d 상태", start, end)))
	fmt.Fprintf(a.Out, "  %-7s  %-6s  %-12s  %-22s  %-7s  %s\n",
		headerStyle("PORT"), headerStyle("PROTO"), headerStyle("STATE"),
		headerStyle("ADDRESS"), headerStyle("PID"), headerStyle("NAME"))
	fmt.Fprintln(a.Out, "  "+strings.Repeat("─", 68))
	for _, res := range occupied {
		for _, b := range res.Bindings {
			fmt.Fprintf(a.Out, "  %-7d  %-6s  %-12s  %-22s  %-7d  %s\n",
				b.Port, b.Proto, stateOrDash(b.State), b.Address(), b.PID, b.Name)
		}
	}
}

// stateOrDash는 빈 상태 문자열을 "-"로 대체합니다 (UDP는 상태가 없는 경우가 많음).
func stateOrDash(state string) string {
	if state == "" {
		return "-"
	}
	return state
}

// ─── 원격 확인 ───────────────────────────────────────────────────────────────

// runRemote는 host:port 원격 도달성을 확인해 결과를 출력합니다.
func (a *checkApp) runRemote(start, end uint16) error {
	if start == end {
		a.logProgress("%s %s:%d %s %s\n",
			headerStyle("🔍"), a.Host, start, dimStyle("("+a.Timeout.String()+")"), warnStyle("도달성을 확인 중입니다..."))
	} else {
		a.logProgress("%s %s %s%d-%d%s %s %s\n",
			headerStyle("🔍"), a.Host, warnStyle("["), start, end, warnStyle("]"),
			dimStyle("("+a.Timeout.String()+")"), warnStyle("도달성을 확인 중입니다..."))
	}

	results := a.checkRemoteRange(start, end)

	if a.JSON {
		if err := a.printRemoteJSON(results); err != nil {
			return err
		}
	} else {
		a.printRemote(results)
	}

	if anyOpen(results) {
		return nil
	}
	return &silentExitError{code: ExitCodeError}
}

// anyOpen은 도달 가능(open)한 포트가 하나라도 있는지 반환합니다.
func anyOpen(results []*portpkg.ReachResult) bool {
	for _, r := range results {
		if r.Status.Open() {
			return true
		}
	}
	return false
}

// checkRemoteRange는 start~end 범위를 최대 checkConcurrency개까지 병렬로 확인하고
// 포트 오름차순으로 결과를 반환합니다. 결과 슬라이스는 인덱스로만 채워 데이터 경합이 없습니다.
func (a *checkApp) checkRemoteRange(start, end uint16) []*portpkg.ReachResult {
	ports := make([]uint16, 0, int(end)-int(start)+1)
	for p := uint32(start); p <= uint32(end); p++ {
		ports = append(ports, uint16(p))
	}

	results := make([]*portpkg.ReachResult, len(ports))
	sem := make(chan struct{}, checkConcurrency)
	var wg sync.WaitGroup

	for i, p := range ports {
		wg.Add(1)
		go func(idx int, port uint16) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			results[idx] = a.Ops.CheckRemote(context.Background(), a.Host, port)
		}(i, p)
	}
	wg.Wait()

	return results
}

// printRemote는 도달성 결과를 한 줄씩 출력하고 요약을 stderr로 보냅니다.
func (a *checkApp) printRemote(results []*portpkg.ReachResult) {
	openCount := 0
	for _, r := range results {
		if r.Status.Open() {
			openCount++
		}
		icon, style := reachStyle(r.Status)
		detail := ""
		if r.Detail != "" {
			detail = " " + dimStyle("("+r.Detail+")")
		}
		fmt.Fprintf(a.Out, "%s %s:%d → %s%s\n",
			icon, r.Host, r.Port, style(string(r.Status)), detail)
	}

	a.logProgress("%s 확인 %d개 포트 중 열린 포트: %d개 (지연 시간은 연결 시도 기준)\n",
		dimStyle("ℹ️"), len(results), openCount)
}

// reachStyle은 도달성 상태에 맞는 아이콘과 색상 함수를 반환합니다.
func reachStyle(s portpkg.ReachStatus) (string, func(...any) string) {
	switch s {
	case portpkg.ReachOpen:
		return "✅", successStyle
	case portpkg.ReachClosed, portpkg.ReachFiltered, portpkg.ReachUnreachable:
		return "⚠️", warnStyle
	default:
		return "❌", errorStyle
	}
}

// ─── JSON 출력 ───────────────────────────────────────────────────────────────

// printLocalJSON은 로컬 확인 결과를 JSON 배열로 출력합니다 (바인딩이 있는 포트만 포함).
func (a *checkApp) printLocalJSON(results []*portpkg.LocalPortResult) error {
	type bindingEntry struct {
		Proto   string `json:"proto"`
		State   string `json:"state,omitempty"`
		Address string `json:"address"`
		Port    uint16 `json:"port"`
		PID     int32  `json:"pid"`
		Name    string `json:"name"`
	}
	type entry struct {
		Port      uint16         `json:"port"`
		Listening bool           `json:"listening"`
		Bindings  []bindingEntry `json:"bindings"`
	}

	entries := make([]entry, 0, len(results))
	for _, r := range results {
		if len(r.Bindings) == 0 {
			continue
		}
		bindings := make([]bindingEntry, 0, len(r.Bindings))
		for _, b := range r.Bindings {
			bindings = append(bindings, bindingEntry{
				Proto:   b.Proto,
				State:   b.State,
				Address: b.Address(),
				Port:    b.Port,
				PID:     b.PID,
				Name:    b.Name,
			})
		}
		entries = append(entries, entry{Port: r.Port, Listening: r.Listening, Bindings: bindings})
	}
	return encodeJSON(a.Out, entries)
}

// printRemoteJSON은 원격 확인 결과를 JSON 배열로 출력합니다 (확인한 모든 포트 포함).
func (a *checkApp) printRemoteJSON(results []*portpkg.ReachResult) error {
	type entry struct {
		Host      string `json:"host"`
		Port      uint16 `json:"port"`
		Status    string `json:"status"`
		Open      bool   `json:"open"`
		LatencyMs int64  `json:"latency_ms"`
		Detail    string `json:"detail,omitempty"`
	}

	entries := make([]entry, 0, len(results))
	for _, r := range results {
		entries = append(entries, entry{
			Host:      r.Host,
			Port:      r.Port,
			Status:    string(r.Status),
			Open:      r.Status.Open(),
			LatencyMs: r.Latency.Milliseconds(),
			Detail:    r.Detail,
		})
	}
	return encodeJSON(a.Out, entries)
}

// encodeJSON은 들여쓴 JSON을 출력합니다 (대상이 없으면 []를 출력해 파이프라인을 보호).
func encodeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
