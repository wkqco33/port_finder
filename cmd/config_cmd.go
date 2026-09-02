// config_cmd.go는 `poff config` 서브커맨드(show/init/set)를 구현합니다.
// 설정 파일 IO는 pkg/config에 위임하고, 이 파일은 입출력과 흐름만 담당합니다.
package cmd

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	cfgpkg "port_finder/pkg/config"

	"github.com/wkqco33/wcli"
)

// parseDurationOrZero는 duration 문자열을 파싱하며 실패 시 0을 반환합니다.
// 유효성 검증은 cfgpkg.Set에서 수행되므로 표시용 오버라이드에는 0(무시)으로 처리합니다.
func parseDurationOrZero(s string) time.Duration {
	d, _ := time.ParseDuration(s)
	return d
}

// cfgApp은 config 서브커맨드가 갖는 외부 의존성(스트림 + 파일 경로)입니다.
// Path/Override를 주입할 수 있어 단위 테스트에서 격리 검증이 가능합니다.
type cfgApp struct {
	Out  io.Writer
	ErrW io.Writer

	// Path는 설정 파일 경로입니다 (테스트에서 임시 경로 주입).
	Path string
	// HomeDir는 홈 디렉터리 조회 함수입니다 (테스트 격리용).
	HomeDir func() (string, error)
	// Override는 CLI 플래그로 덮어쓸 설정 값입니다 (key: "model"|"base_url"|"timeout").
	Override map[string]string
}

// newConfigCommand는 `poff config` 서브커맨드(show/init/set)를 생성합니다.
func newConfigCommand() *wcli.Command {
	cfgCmd := &wcli.Command{
		Use:   "config",
		Short: "poff 설정 관리 (show/init/set)",
		Long: `poff의 AI 분석 설정(~/.poff.json)을 관리합니다.

사용 예:
  poff config show                     현재 유효 설정과 출처를 표시
  poff config init                     기본값 설정 파일 생성
  poff config set ai.model qwen3:4b    모델 변경
  poff config set ai.timeout 90s       타임아웃 변경

설정 우선순위: CLI 플래그 > 설정 파일 > 기본값`,
		// Run이 없으면 wcli가 서브커맨드 목록과 함께 도움말을 출력합니다.
	}

	cfgCmd.AddCommand(
		&wcli.Command{
			Use:   "show",
			Short: "현재 유효 설정 표시 (출처 포함)",
			Run: func(ctx *wcli.Context) error {
				return newRealCfgApp().runShow()
			},
		},
		&wcli.Command{
			Use:   "init",
			Short: "기본값 설정 파일 생성 (~/.poff.json)",
			Run: func(ctx *wcli.Context) error {
				return newRealCfgApp().runInit()
			},
		},
		&wcli.Command{
			Use:   "set KEY VALUE",
			Short: "설정 값 변경 (예: poff config set ai.model llama3.2)",
			Run: func(ctx *wcli.Context) error {
				if len(ctx.Args) != 2 {
					return fmt.Errorf("set은 KEY VALUE 2개의 인자가 필요합니다 (사용: poff config set ai.model qwen3:4b)")
				}
				return newRealCfgApp().runSet(ctx.Args[0], ctx.Args[1])
			},
		},
	)
	return cfgCmd
}

// newRealCfgApp은 실제 홈 디렉터리와 표준 스트림을 사용하는 cfgApp을 구성합니다.
func newRealCfgApp() *cfgApp {
	return &cfgApp{
		Out:      os.Stdout,
		ErrW:     os.Stderr,
		HomeDir:  os.UserHomeDir,
		Override: map[string]string{},
	}
}

// resolvePath는 테스트 주입 Path가 있으면 그것을, 없으면 실제 홈 경로를 사용합니다.
func (c *cfgApp) resolvePath() (string, error) {
	if c.Path != "" {
		return c.Path, nil
	}
	return cfgpkg.ResolvePath(c.HomeDir)
}

// applyOverride는 설정 값에 CLI 플래그 오버라이드를 적용한 복사본을 반환합니다.
func (c *cfgApp) applyOverride(ai cfgpkg.AIConfig) cfgpkg.AIConfig {
	if v, ok := c.Override["model"]; ok && v != "" {
		ai.Model = v
	}
	if v, ok := c.Override["base_url"]; ok && v != "" {
		ai.BaseURL = v
	}
	if v, ok := c.Override["timeout"]; ok && v != "" {
		ai.Timeout = cfgpkg.Duration(parseDurationOrZero(v))
	}
	return ai
}

// RunConfig는 config 서브커맨드를 디스패치합니다.
// args는 "show"/"init"/"set KEY VALUE" 형태의 인자 목록입니다.
func (c *cfgApp) RunConfig(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("서브커맨드가 필요합니다 (사용: poff config [show|init|set KEY VALUE])")
	}

	switch args[0] {
	case "show":
		if len(args) > 1 {
			return fmt.Errorf("show는 인자를 받지 않습니다 (사용: poff config show)")
		}
		return c.runShow()
	case "init":
		if len(args) > 1 {
			return fmt.Errorf("init은 인자를 받지 않습니다 (사용: poff config init)")
		}
		return c.runInit()
	case "set":
		if len(args) != 3 {
			return fmt.Errorf("set은 KEY VALUE 2개의 인자가 필요합니다 (사용: poff config set ai.model qwen3:4b)")
		}
		return c.runSet(args[1], args[2])
	default:
		return fmt.Errorf("알 수 없는 서브커맨드 %q (사용 가능: show, init, set)", args[0])
	}
}

// runShow는 현재 유효 설정(출처 포함)을 출력합니다.
func (c *cfgApp) runShow() error {
	path, err := c.resolvePath()
	if err != nil {
		return err
	}

	cfg, err := cfgpkg.Load(path)
	if err != nil {
		return err
	}

	effective := c.applyOverride(cfgpkg.Apply(cfg.AI, cfgpkg.AIConfig{}))
	srcModel := c.sourceOf(cfg.AI.Model != "", "model")
	srcURL := c.sourceOf(cfg.AI.BaseURL != "", "base_url")
	srcTO := c.sourceOf(cfg.AI.Timeout > 0, "timeout")

	fmt.Fprintf(c.Out, "%s %s %s\n\n", headerStyle("⚙️"), headerStyle("poff 설정"), dimStyle("("+path+")"))
	fmt.Fprintf(c.Out, "  %s %s  %s\n", keyStyle("model"), effective.Model, dimStyle("("+srcModel+")"))
	fmt.Fprintf(c.Out, "  %s %s  %s\n", keyStyle("base_url"), effective.BaseURL, dimStyle("("+srcURL+")"))
	fmt.Fprintf(c.Out, "  %s %s  %s\n", keyStyle("timeout"), time.Duration(effective.Timeout), dimStyle("("+srcTO+")"))
	fmt.Fprintln(c.Out)
	fmt.Fprintln(c.Out, dimStyle("변경: poff config set <KEY> <VALUE> (KEY: "+strings.Join(cfgKeys(), ", ")+")"))
	return nil
}

// sourceOf는 해당 항목의 설정 출처 라벨을 반환합니다 (플래그 > 파일 > 기본값).
func (c *cfgApp) sourceOf(fileSet bool, key string) string {
	if v, ok := c.Override[key]; ok && v != "" {
		return "플래그"
	}
	if fileSet {
		return "설정 파일"
	}
	return "기본값"
}

// runInit는 기본값 설정 파일을 생성합니다.
func (c *cfgApp) runInit() error {
	path, err := c.resolvePath()
	if err != nil {
		return err
	}

	if err := cfgpkg.Init(path); err != nil {
		return err
	}

	fmt.Fprintf(c.Out, "%s 설정 파일이 생성되었습니다: %s\n", successStyle("✅"), valueStyle(path))
	fmt.Fprintf(c.Out, "%s %s\n", dimStyle("확인:"), "poff config show")
	return nil
}

// runSet는 단일 키 값을 변경하고 결과를 출력합니다.
func (c *cfgApp) runSet(key, value string) error {
	path, err := c.resolvePath()
	if err != nil {
		return err
	}

	if err := cfgpkg.Set(path, key, value); err != nil {
		return err
	}

	fmt.Fprintf(c.Out, "%s %s %s %s\n", successStyle("✅"), keyStyle(key), warnStyle("→"), valueStyle(value))
	fmt.Fprintf(c.Out, "%s %s\n", dimStyle("저장:"), dimStyle(path))
	return nil
}

// cfgKeys는 사용 가능한 설정 키 목록을 정렬해 반환합니다 (도움말 표시용).
func cfgKeys() []string {
	keys := []string{"ai.model", "ai.base_url", "ai.timeout"}
	sort.Strings(keys)
	return keys
}
