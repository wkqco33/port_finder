package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"port-finder/pkg/port"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var Version = "1.0.0"

var (
	portStr   string
	forceKill bool
	listMode  bool
	jsonOut   bool
	graceful  bool
)

var (
	headerStyle  = color.New(color.FgCyan, color.Bold).SprintFunc()
	successStyle = color.New(color.FgGreen, color.Bold).SprintFunc()
	warnStyle    = color.New(color.FgYellow).SprintFunc()
	errorStyle   = color.New(color.FgRed, color.Bold).SprintFunc()
	keyStyle     = color.New(color.FgHiBlue).SprintFunc()
	valueStyle   = color.New(color.FgWhite).SprintFunc()
	promptStyle  = color.New(color.FgHiMagenta, color.Bold).SprintFunc()
	dimStyle     = color.New(color.Faint).SprintFunc()
)

var rootCmd = &cobra.Command{
	Use:     "port_finder",
	Version: Version,
	Short:   "포트를 사용하는 프로세스를 찾아 종료하는 유틸리티",
	Long: `지정한 포트 번호를 사용하는 프로세스의 정보를 보여주고,
사용자 확인 후 해당 프로세스를 즉시 종료할 수 있습니다.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if listMode {
			return runList()
		}
		if portStr == "" {
			return cmd.Help()
		}
		start, end, err := ParsePortArg(portStr)
		if err != nil {
			return err
		}
		if start == end {
			return runSinglePort(start)
		}
		return runPortRange(start, end)
	},
}

// ParsePortArg는 "8080" 또는 "3000-4000" 형식의 문자열을 파싱합니다.
func ParsePortArg(s string) (start, end uint16, err error) {
	parts := strings.SplitN(s, "-", 2)
	if len(parts) == 1 {
		n, e := strconv.ParseUint(parts[0], 10, 16)
		if e != nil || n == 0 {
			return 0, 0, fmt.Errorf("유효하지 않은 포트 번호: %s", s)
		}
		return uint16(n), uint16(n), nil
	}
	lo, e1 := strconv.ParseUint(parts[0], 10, 16)
	hi, e2 := strconv.ParseUint(parts[1], 10, 16)
	if e1 != nil || e2 != nil || lo == 0 || hi == 0 || lo > hi {
		return 0, 0, fmt.Errorf("유효하지 않은 포트 범위: %s (예: 3000-4000)", s)
	}
	return uint16(lo), uint16(hi), nil
}

// ─── 실행 흐름 ────────────────────────────────────────────────────────────────

func runList() error {
	infos, err := port.ListAll()
	if err != nil {
		return err
	}
	if len(infos) == 0 {
		fmt.Println(warnStyle("ℹ️  사용 중인 포트가 없습니다."))
		return nil
	}
	if jsonOut {
		return printJSON(infos)
	}
	fmt.Printf("%s %s\n\n", headerStyle("📋"), headerStyle("현재 사용 중인 포트 목록"))
	printTable(infos)
	return nil
}

func runSinglePort(p uint16) error {
	fmt.Printf("%s %s %d %s\n",
		headerStyle("🔍"), warnStyle("포트"), p, warnStyle("사용 중인 프로세스를 검색 중입니다..."))

	info, err := port.FindByPort(p)
	if err != nil {
		return err
	}
	if info == nil {
		fmt.Printf("%s 포트 %d를 사용하는 프로세스를 찾을 수 없습니다.\n", warnStyle("⚠️"), p)
		return nil
	}

	if jsonOut {
		return printJSON([]*port.ProcessInfo{info})
	}

	fmt.Println()
	fmt.Printf("%s %s\n", successStyle("✨ 발견!"), valueStyle("프로세스 상세 정보"))
	fmt.Printf("   %s %-10s : %s\n", keyStyle("•"), "PID", valueStyle(fmt.Sprintf("%d", info.PID)))
	fmt.Printf("   %s %-10s : %s\n", keyStyle("•"), "NAME", valueStyle(info.Name))
	fmt.Printf("   %s %-10s : %s\n", keyStyle("•"), "PORT", valueStyle(fmt.Sprintf("%d", info.Port)))
	fmt.Println()

	if !forceKill {
		fmt.Printf("%s %s %s",
			promptStyle("🔥"),
			errorStyle("해당 프로세스를 즉시 종료하시겠습니까?"),
			warnStyle("(y/N): "))
		if !readConfirm() {
			fmt.Printf("\n%s %s\n", warnStyle("ℹ️"), valueStyle("프로세스 종료가 취소되었습니다."))
			return nil
		}
	}

	if err := killWith(info.PID); err != nil {
		fmt.Fprintf(os.Stderr, "\n%s %v\n", errorStyle("❌ 종료 실패:"), err)
		os.Exit(1)
	}
	fmt.Printf("\n%s PID %d %s\n", successStyle("✅"), info.PID, valueStyle("프로세스가 안전하게 종료되었습니다."))
	return nil
}

func runPortRange(start, end uint16) error {
	fmt.Printf("%s 포트 %s%d-%d%s 범위를 스캔 중입니다...\n",
		headerStyle("🔍"), warnStyle("["), start, end, warnStyle("]"))

	infos, err := port.FindByPortRange(start, end)
	if err != nil {
		return err
	}
	if len(infos) == 0 {
		fmt.Printf("%s 포트 %d-%d 범위에서 사용 중인 포트를 찾을 수 없습니다.\n", warnStyle("⚠️"), start, end)
		return nil
	}

	if jsonOut {
		return printJSON(infos)
	}

	fmt.Println()
	printTable(infos)
	fmt.Println()

	if forceKill {
		fmt.Printf("%s 총 %d개 프로세스를 강제 종료합니다...\n", warnStyle("🔥"), len(infos))
		for _, info := range infos {
			if err := killWith(info.PID); err != nil {
				fmt.Fprintf(os.Stderr, "%s PID %d 종료 실패: %v\n", errorStyle("❌"), info.PID, err)
			} else {
				fmt.Printf("%s PID %d (%s:%d) 종료 완료\n", successStyle("✅"), info.PID, info.Name, info.Port)
			}
		}
	} else {
		fmt.Printf("%s\n", dimStyle("특정 포트를 종료하려면: port_finder -p <PORT> [-f]"))
	}
	return nil
}

// ─── 출력 헬퍼 ────────────────────────────────────────────────────────────────

func printTable(infos []*port.ProcessInfo) {
	fmt.Printf("  %-8s  %-10s  %s\n", headerStyle("PORT"), headerStyle("PID"), headerStyle("NAME"))
	fmt.Println("  " + strings.Repeat("─", 36))
	for _, info := range infos {
		fmt.Printf("  %-8d  %-10d  %s\n", info.Port, info.PID, info.Name)
	}
}

func printJSON(infos []*port.ProcessInfo) error {
	type entry struct {
		Port uint16 `json:"port"`
		PID  int32  `json:"pid"`
		Name string `json:"name"`
	}
	entries := make([]entry, len(infos))
	for i, info := range infos {
		entries[i] = entry{Port: info.Port, PID: info.PID, Name: info.Name}
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(entries)
}

// ─── 종료 헬퍼 ────────────────────────────────────────────────────────────────

func killWith(pid int32) error {
	if graceful {
		return port.KillProcessGracefully(pid, 5*time.Second)
	}
	return port.KillProcessByPID(pid)
}

func readConfirm() bool {
	reader := bufio.NewReader(os.Stdin)
	answer, _ := reader.ReadString('\n')
	return strings.ToLower(strings.TrimSpace(answer)) == "y"
}

// ─── Cobra 설정 ───────────────────────────────────────────────────────────────

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.Flags().StringVarP(&portStr, "port", "p", "", "검색할 포트 번호 또는 범위 (예: 8080, 3000-4000)")
	rootCmd.Flags().BoolVarP(&forceKill, "force", "f", false, "확인 없이 즉시 프로세스 종료")
	rootCmd.Flags().BoolVarP(&listMode, "list", "l", false, "현재 사용 중인 모든 포트 목록 출력")
	rootCmd.Flags().BoolVarP(&jsonOut, "json", "j", false, "JSON 형식으로 출력")
	rootCmd.Flags().BoolVarP(&graceful, "graceful", "g", false, "SIGTERM 후 5초 대기, 이후 SIGKILL (Graceful 종료)")
	rootCmd.Flags().BoolP("version", "v", false, "버전 출력")
}
