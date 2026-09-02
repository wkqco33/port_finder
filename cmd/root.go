package cmd

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"port_finder/pkg/ai"
	cfgpkg "port_finder/pkg/config"
	"port_finder/pkg/port"

	"github.com/fatih/color"
	"github.com/wkqco33/wcli"
)

var Version = "1.0.0"

var (
	portStr    string
	forceKill  bool
	listMode   bool
	jsonOut    bool
	graceful   bool
	aiMode     bool
	aiModel    string
	aiBaseURL  string
	cfgTimeout time.Duration
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

// portOps는 CLI가 사용하는 port 패키지의 표면입니다.
// 테스트에서 페이크로 대체할 수 있어 cmd 로직을 격리해서 검증할 수 있습니다.
type portOps interface {
	FindByPort(target uint16) ([]*port.ProcessInfo, error)
	FindByPortRange(start, end uint16) ([]*port.ProcessInfo, error)
	ListAll() ([]*port.ProcessInfo, error)
	KillProcessByPID(pid int32) error
	KillProcessGracefully(pid int32, timeout time.Duration) error
}

// aiOps는 CLI가 사용하는 ai 패키지의 표면입니다.
// 테스트에서 페이크로 대체할 수 있습니다.
type aiOps interface {
	Analyze(ctx context.Context, services []ai.Service) (string, error)
}

// App은 CLI가 갖는 모든 외부 의존성(입출력 스트림 + 포트 연산)을 한곳에 모은 구조체입니다.
// 스트림과 ops를 주입할 수 있어 단위 테스트에서 bytes.Buffer/페이크로 검증할 수 있습니다.
type App struct {
	Out  io.Writer
	ErrW io.Writer
	In   io.Reader

	Ops portOps
	AI  aiOps

	PortStr   string
	ForceKill bool
	ListMode  bool
	JSON      bool
	Graceful  bool
	AIMode    bool
	AIModel   string
}

// newApp은 cobra 플래그와 실제 프로세스 스트림으로 App을 구성합니다.
// AI 설정은 우선순위(CLI 플래그 > 설정 파일 > 기본값)에 따라 구성됩니다.
func newApp() *App {
	aiCfg := resolveAIConfig()

	return &App{
		Out:  os.Stdout,
		ErrW: os.Stderr,
		In:   os.Stdin,
		Ops:  port.NewFinder(),
		AI: ai.NewAnalyzer(
			ai.WithModel(aiCfg.Model),
			ai.WithBaseURL(aiCfg.BaseURL),
			ai.WithTimeout(time.Duration(aiCfg.Timeout)),
		),

		PortStr:   portStr,
		ForceKill: forceKill,
		ListMode:  listMode,
		JSON:      jsonOut,
		Graceful:  graceful,
		AIMode:    aiMode,
		AIModel:   aiCfg.Model,
	}
}

// resolveAIConfig는 설정 파일을 로드해 CLI 플래그 오버라이드를 적용한
// 최종 AI 설정을 반환합니다. 설정 파일이 없거나 손상되면 기본값으로 폴백합니다.
func resolveAIConfig() cfgpkg.AIConfig {
	var flags cfgpkg.AIConfig
	if aiModel != "" {
		flags.Model = aiModel
	}
	if aiBaseURL != "" {
		flags.BaseURL = aiBaseURL
	}
	if cfgTimeout > 0 {
		flags.Timeout = cfgpkg.Duration(cfgTimeout)
	}

	path, err := cfgpkg.ResolvePath(os.UserHomeDir)
	if err != nil {
		return cfgpkg.Apply(cfgpkg.AIConfig{}, flags)
	}
	fileCfg, err := cfgpkg.Load(path)
	if err != nil {
		// 손상된 설정은 조용히 기본값으로 폴백합니다 (CLI가 막히지 않도록).
		return cfgpkg.Apply(cfgpkg.AIConfig{}, flags)
	}
	return cfgpkg.Apply(fileCfg.AI, flags)
}

// Run은 플래그 상태에 따라 목록/단일/범위/AI 실행을 디스패치합니다.
// help는 인자가 없을 때 호출할 헬프 함수입니다 (테스트에서는 무시 가능한 함수 전달).
// AI 분석은 --ai 플래그(AIMode)가 켜진 경우에만 실행됩니다.
func (a *App) Run(help func() error) error {
	if a.AIMode {
		// AI 모드는 목록 기반 분석 전용이므로 -p와 동시 사용을 막습니다.
		if a.PortStr != "" {
			return errors.New("--ai는 포트 목록 분석 전용입니다. -p/--port와 함께 사용할 수 없습니다 (단독으로 실행하세요)")
		}
		// --json은 구조화된 데이터 출력용이므로 자유 텍스트인 AI 분석과 동시 사용을 막습니다.
		if a.JSON {
			return errors.New("--ai는 자유 텍스트 분석을 출력하므로 --json과 함께 사용할 수 없습니다")
		}
		return a.runAI()
	}
	if a.ListMode {
		return a.runList()
	}
	if a.PortStr == "" {
		return help()
	}
	start, end, err := ParsePortArg(a.PortStr)
	if err != nil {
		return err
	}
	if start == end {
		return a.runSinglePort(start)
	}
	return a.runPortRange(start, end)
}

var rootCmd = &wcli.Command{
	Use:     "poff",
	Version: Version,
	Short:   "포트를 사용하는 프로세스를 찾아 종료하는 유틸리티",
	Long: `지정한 포트 번호를 사용하는 프로세스의 정보를 보여주고,
사용자 확인 후 해당 프로세스를 즉시 종료할 수 있습니다.

--ai 옵션으로 현재 사용 중인 포트 전체를 LLM(Ollama)에게 분석시켜
서비스 용도 추정, 위험도, 정리 제안을 받을 수 있습니다.`,
	// SilenceErrors: wcli의 기본 에러 출력을 끄고 Execute()에서 직접 처리합니다.
	SilenceErrors: true,
}

// ParsePortArg는 "8080" 또는 "3000-4000" 형식의 문자열을 파싱합니다.
func ParsePortArg(s string) (start, end uint16, err error) {
	s = strings.TrimSpace(s)
	parts := strings.SplitN(s, "-", 2)
	if len(parts) == 1 {
		n, e := strconv.ParseUint(strings.TrimSpace(parts[0]), 10, 16)
		if e != nil || n == 0 {
			return 0, 0, fmt.Errorf("유효하지 않은 포트 번호: %s", s)
		}
		return uint16(n), uint16(n), nil
	}
	lo, e1 := strconv.ParseUint(strings.TrimSpace(parts[0]), 10, 16)
	hi, e2 := strconv.ParseUint(strings.TrimSpace(parts[1]), 10, 16)
	if e1 != nil || e2 != nil || lo == 0 || hi == 0 || lo > hi {
		return 0, 0, fmt.Errorf("유효하지 않은 포트 범위: %s (예: 3000-4000)", s)
	}
	return uint16(lo), uint16(hi), nil
}

// ─── 실행 흐름 ────────────────────────────────────────────────────────────────

func (a *App) runList() error {
	infos, err := a.Ops.ListAll()
	if err != nil {
		return err
	}
	if len(infos) == 0 {
		fmt.Fprintln(a.Out, warnStyle("ℹ️  사용 중인 포트가 없습니다."))
		return nil
	}
	if a.JSON {
		return a.printJSON(infos)
	}
	fmt.Fprintf(a.Out, "%s %s\n\n", headerStyle("📋"), headerStyle("현재 사용 중인 포트 목록"))
	a.printTable(infos)
	return nil
}

func (a *App) runSinglePort(p uint16) error {
	fmt.Fprintf(a.Out, "%s %s %d %s\n",
		headerStyle("🔍"), warnStyle("포트"), p, warnStyle("사용 중인 프로세스를 검색 중입니다..."))

	infos, err := a.Ops.FindByPort(p)
	if err != nil {
		return err
	}
	if len(infos) == 0 {
		fmt.Fprintf(a.Out, "%s 포트 %d를 사용하는 프로세스를 찾을 수 없습니다.\n", warnStyle("⚠️"), p)
		return nil
	}

	if a.JSON {
		return a.printJSON(infos)
	}

	fmt.Fprintln(a.Out)
	fmt.Fprintf(a.Out, "%s %s\n", successStyle("✨ 발견!"), valueStyle("프로세스 상세 정보"))
	for _, info := range infos {
		fmt.Fprintf(a.Out, "   %s %-10s : %s\n", keyStyle("•"), "PID", valueStyle(fmt.Sprintf("%d", info.PID)))
		fmt.Fprintf(a.Out, "   %s %-10s : %s\n", keyStyle("•"), "NAME", valueStyle(info.Name))
		fmt.Fprintf(a.Out, "   %s %-10s : %s\n", keyStyle("•"), "PORT", valueStyle(fmt.Sprintf("%d", info.Port)))
		fmt.Fprintln(a.Out)
	}

	if !a.ForceKill {
		fmt.Fprintf(a.Out, "%s %s %s",
			promptStyle("🔥"),
			errorStyle("해당 프로세스들을 즉시 종료하시겠습니까?"),
			warnStyle("(y/N): "))
		if !a.readConfirm() {
			fmt.Fprintf(a.Out, "\n%s %s\n", warnStyle("ℹ️"), valueStyle("프로세스 종료가 취소되었습니다."))
			return nil
		}
	}

	for _, info := range infos {
		if err := a.killWith(info.PID); err != nil {
			fmt.Fprintf(a.ErrW, "\n%s PID %d %v\n", errorStyle("❌ 종료 실패:"), info.PID, err)
		} else {
			fmt.Fprintf(a.Out, "\n%s PID %d %s\n", successStyle("✅"), info.PID, valueStyle("프로세스가 안전하게 종료되었습니다."))
		}
	}
	return nil
}

func (a *App) runPortRange(start, end uint16) error {
	fmt.Fprintf(a.Out, "%s 포트 %s%d-%d%s 범위를 스캔 중입니다...\n",
		headerStyle("🔍"), warnStyle("["), start, end, warnStyle("]"))

	infos, err := a.Ops.FindByPortRange(start, end)
	if err != nil {
		return err
	}
	if len(infos) == 0 {
		fmt.Fprintf(a.Out, "%s 포트 %d-%d 범위에서 사용 중인 포트를 찾을 수 없습니다.\n", warnStyle("⚠️"), start, end)
		return nil
	}

	if a.JSON {
		return a.printJSON(infos)
	}

	fmt.Fprintln(a.Out)
	a.printTable(infos)
	fmt.Fprintln(a.Out)

	if a.ForceKill {
		fmt.Fprintf(a.Out, "%s 총 %d개 프로세스를 강제 종료합니다...\n", warnStyle("🔥"), len(infos))
		for _, info := range infos {
			if err := a.killWith(info.PID); err != nil {
				fmt.Fprintf(a.ErrW, "%s PID %d 종료 실패: %v\n", errorStyle("❌"), info.PID, err)
			} else {
				fmt.Fprintf(a.Out, "%s PID %d (%s:%d) 종료 완료\n", successStyle("✅"), info.PID, info.Name, info.Port)
			}
		}
	} else {
		fmt.Fprintf(a.Out, "%s\n", dimStyle("특정 포트를 종료하려면: poff -p <PORT> [-f]"))
	}
	return nil
}

// runAI는 포트 목록을 조회해 테이블로 출력한 뒤 LLM으로 분석합니다.
// AI 모드는 분석 전용으로, 프로세스 종료 흐름으로 진입하지 않습니다.
func (a *App) runAI() error {
	model := a.AIModel
	if model == "" {
		model = ai.DefaultModel
	}

	infos, err := a.Ops.ListAll()
	if err != nil {
		return err
	}
	if len(infos) == 0 {
		fmt.Fprintln(a.Out, warnStyle("ℹ️  사용 중인 포트가 없습니다. 분석할 대상이 없습니다."))
		return nil
	}

	fmt.Fprintf(a.Out, "%s %s\n\n", headerStyle("📋"), headerStyle("현재 사용 중인 포트 목록"))
	a.printTable(infos)
	fmt.Fprintln(a.Out)

	fmt.Fprintf(a.Out, "%s %s 분석 중입니다...\n", headerStyle("🤖"), dimStyle("Ollama("+model+")"))

	// cmd는 pkg/port 데이터만 알므로 pkg/ai 타입으로 변환합니다.
	services := make([]ai.Service, len(infos))
	for i, info := range infos {
		services[i] = ai.Service{Port: info.Port, PID: info.PID, Name: info.Name}
	}

	result, err := a.AI.Analyze(context.Background(), services)
	if err != nil {
		return err
	}

	fmt.Fprintf(a.Out, "%s %s\n\n", successStyle("✨ AI 분석 결과"), dimStyle("("+model+")"))
	fmt.Fprintln(a.Out, result)
	fmt.Fprintln(a.Out)
	fmt.Fprintf(a.Out, "%s %s\n", dimStyle("AI 분석은 참고용이며,"), dimStyle("종료는 poff -p <PORT> [-f] 로 수행하세요."))
	return nil
}

// ─── 출력 헬퍼 ────────────────────────────────────────────────────────────────

func (a *App) printTable(infos []*port.ProcessInfo) {
	fmt.Fprintf(a.Out, "  %-8s  %-10s  %s\n", headerStyle("PORT"), headerStyle("PID"), headerStyle("NAME"))
	fmt.Fprintln(a.Out, "  "+strings.Repeat("─", 36))
	for _, info := range infos {
		fmt.Fprintf(a.Out, "  %-8d  %-10d  %s\n", info.Port, info.PID, info.Name)
	}
}

func (a *App) printJSON(infos []*port.ProcessInfo) error {
	type entry struct {
		Port uint16 `json:"port"`
		PID  int32  `json:"pid"`
		Name string `json:"name"`
	}
	entries := make([]entry, len(infos))
	for i, info := range infos {
		entries[i] = entry{Port: info.Port, PID: info.PID, Name: info.Name}
	}
	enc := json.NewEncoder(a.Out)
	enc.SetIndent("", "  ")
	return enc.Encode(entries)
}

// ─── 종료 헬퍼 ────────────────────────────────────────────────────────────────

func (a *App) killWith(pid int32) error {
	if a.Graceful {
		return a.Ops.KillProcessGracefully(pid, 5*time.Second)
	}
	return a.Ops.KillProcessByPID(pid)
}

func (a *App) readConfirm() bool {
	reader := bufio.NewReader(a.In)
	answer, _ := reader.ReadString('\n')
	return strings.ToLower(strings.TrimSpace(answer)) == "y"
}

// ─── Cobra 설정 ───────────────────────────────────────────────────────────────

func Execute() {
	if err := rootCmd.Execute(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	if Version == "1.0.0" {
		Version = resolveVersion()
	}
	rootCmd.Version = Version

	// Run은 rootCmd를 참조하므로 초기화 순환을 피하기 위해 여기서 할당합니다.
	rootCmd.Run = func(ctx *wcli.Context) error {
		// 인자 없이 실행된 경우 루트 도움말을 출력합니다.
		return newApp().Run(func() error {
			rootCmd.Help()
			return nil
		})
	}

	rootCmd.Flags().StringVar(&portStr, "port", "p", "", "검색할 포트 번호 또는 범위 (예: 8080, 3000-4000)")
	rootCmd.Flags().BoolVar(&forceKill, "force", "f", false, "확인 없이 즉시 프로세스 종료")
	rootCmd.Flags().BoolVar(&listMode, "list", "l", false, "현재 사용 중인 모든 포트 목록 출력")
	rootCmd.Flags().BoolVar(&jsonOut, "json", "j", false, "JSON 형식으로 출력")
	rootCmd.Flags().BoolVar(&graceful, "graceful", "g", false, "SIGTERM 후 5초 대기, 이후 SIGKILL (Graceful 종료)")
	rootCmd.Flags().BoolVar(&aiMode, "ai", "a", false, "LLM(Ollama)으로 현재 사용 중 포트를 분석 (목록 분석 전용)")
	rootCmd.Flags().StringVar(&aiModel, "ai-model", "", "", "AI 분석에 사용할 Ollama 모델 (기본: 설정값 또는 qwen3:4b)")
	rootCmd.Flags().StringVar(&aiBaseURL, "ai-base-url", "", "", "AI 분석에 사용할 LLM 엔드포인트 (기본: 설정값 또는 http://localhost:11434/v1)")
	rootCmd.Flags().DurationVar(&cfgTimeout, "ai-timeout", "", 0, "AI 분석 요청 타임아웃 (기본: 설정값 또는 1m, 예: 90s)")

	// config 서브커맨드 (show/init/set)를 등록합니다.
	rootCmd.AddCommand(newConfigCommand())

	// wcli는 Version 필드가 설정되면 --version 플래그를 자동으로 등록합니다.
}

func resolveVersion() string {
	// 1. debug.ReadBuildInfo()를 통해 go install 등으로 설치된 경우의 버전 확인
	if info, ok := debug.ReadBuildInfo(); ok {
		if info.Main.Version != "" && info.Main.Version != "(devel)" {
			return info.Main.Version
		}
	}

	// 2. git describe --tags --always --dirty 실행 시도
	if gitVer, err := getVersionFromGit(); err == nil && gitVer != "" {
		return gitVer
	}

	// 3. 깃이 없거나 실패한 경우, debug.ReadBuildInfo()의 vcs.revision 정보를 기반으로 표시
	if info, ok := debug.ReadBuildInfo(); ok {
		var revision string
		var modified bool
		for _, setting := range info.Settings {
			switch setting.Key {
			case "vcs.revision":
				revision = setting.Value
			case "vcs.modified":
				modified = setting.Value == "true"
			}
		}
		if revision != "" {
			if len(revision) > 7 {
				revision = revision[:7]
			}
			if modified {
				revision += "-dirty"
			}
			return "dev-" + revision
		}
	}

	// 4. 모두 실패하면 기본값 반환
	return "1.0.0"
}

func getVersionFromGit() (string, error) {
	cmd := exec.Command("git", "describe", "--tags", "--always", "--dirty")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
