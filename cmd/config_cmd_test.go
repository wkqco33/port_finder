// config_cmd_test.go는 `poff config` 서브커맨드(show/init/set)의 hermetic 단위 테스트입니다.
// 파일 IO는 t.TempDir()로 격리하고, 출력은 bytes.Buffer로 검증합니다.
package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// newCfgAppForTest는 임시 경로를 사용하는 cfgApp을 생성합니다.
func newCfgAppForTest(out *bytes.Buffer, path string) *cfgApp {
	return &cfgApp{
		Out:      out,
		ErrW:     out,
		Path:     path,
		HomeDir:  func() (string, error) { return "/nonexistent-home", nil },
		Override: map[string]string{},
	}
}

// ─── config show ──────────────────────────────────────────────────────────────

func TestCfgShow_DefaultsWhenNoFile(t *testing.T) {
	var out bytes.Buffer
	app := newCfgAppForTest(&out, filepath.Join(t.TempDir(), "missing.json"))

	if err := app.runShow(); err != nil {
		t.Fatalf("runShow() error = %v", err)
	}

	s := out.String()
	for _, want := range []string{"qwen3:4b", "http://localhost:11434/v1", "1m0s", "(기본값)"} {
		if !strings.Contains(s, want) {
			t.Errorf("show 출력에 %q 누락:\n%s", want, s)
		}
	}
}

func TestCfgShow_ReflectsFileValues(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".poff.json")
	if err := os.WriteFile(path, []byte(`{"ai":{"model":"llama3.2"}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	app := newCfgAppForTest(&out, path)
	if err := app.runShow(); err != nil {
		t.Fatalf("runShow() error = %v", err)
	}
	if !strings.Contains(out.String(), "llama3.2") {
		t.Errorf("설정 파일 값이 출력되어야 합니다:\n%s", out.String())
	}
}

func TestCfgShow_OverrideFlagTakesPrecedence(t *testing.T) {
	var out bytes.Buffer
	app := newCfgAppForTest(&out, filepath.Join(t.TempDir(), "missing.json"))
	app.Override["model"] = "override-model"

	if err := app.runShow(); err != nil {
		t.Fatalf("runShow() error = %v", err)
	}
	s := out.String()
	if !strings.Contains(s, "override-model") {
		t.Errorf("플래그 값이 출력되어야 합니다:\n%s", s)
	}
	if !strings.Contains(s, "(플래그)") {
		t.Errorf("플래그 출처가 표시되어야 합니다:\n%s", s)
	}
}

// ─── config init ──────────────────────────────────────────────────────────────

func TestCfgInit_CreatesFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".poff.json")
	var out bytes.Buffer
	app := newCfgAppForTest(&out, path)

	if err := app.runInit(); err != nil {
		t.Fatalf("runInit() error = %v", err)
	}

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("설정 파일이 생성되어야 합니다: %v", err)
	}
	if !strings.Contains(out.String(), path) {
		t.Errorf("생성된 경로가 출력되어야 합니다:\n%s", out.String())
	}
}

func TestCfgInit_AlreadyExistsReturnsError(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".poff.json")
	if err := os.WriteFile(path, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	app := newCfgAppForTest(&out, path)
	if err := app.runInit(); err == nil {
		t.Fatal("이미 존재하는 파일이면 에러가 반환되어야 합니다")
	}
}

// ─── config set ───────────────────────────────────────────────────────────────

func TestCfgSet_UpdatesFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".poff.json")
	var out bytes.Buffer
	app := newCfgAppForTest(&out, path)

	if err := app.runSet("ai.model", "qwen3:8b"); err != nil {
		t.Fatalf("runSet() error = %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("설정 파일이 생성되어야 합니다: %v", err)
	}
	if !strings.Contains(string(data), "qwen3:8b") {
		t.Errorf("변경 값이 저장되어야 합니다: %s", data)
	}
	if !strings.Contains(out.String(), "ai.model") || !strings.Contains(out.String(), "qwen3:8b") {
		t.Errorf("변경 결과가 출력되어야 합니다:\n%s", out.String())
	}
}

func TestCfgSet_InvalidKeyReturnsError(t *testing.T) {
	var out bytes.Buffer
	app := newCfgAppForTest(&out, filepath.Join(t.TempDir(), ".poff.json"))

	if err := app.runSet("bad.key", "v"); err == nil {
		t.Error("알 수 없는 키는 에러가 반환되어야 합니다")
	}
}

func TestCfgSet_InvalidTimeoutReturnsError(t *testing.T) {
	var out bytes.Buffer
	app := newCfgAppForTest(&out, filepath.Join(t.TempDir(), ".poff.json"))

	if err := app.runSet("ai.timeout", "abc"); err == nil {
		t.Error("잘못된 duration은 에러가 반환되어야 합니다")
	}
}

// ─── 인자 검증 ────────────────────────────────────────────────────────────────

func TestRunConfig_InvalidSubcommandReturnsError(t *testing.T) {
	var out bytes.Buffer
	app := newCfgAppForTest(&out, filepath.Join(t.TempDir(), "x.json"))

	if err := app.RunConfig([]string{"bogus"}); err == nil {
		t.Error("알 수 없는 서브커맨드는 에러가 반환되어야 합니다")
	}
}

func TestRunConfig_SetRequiresTwoArgs(t *testing.T) {
	var out bytes.Buffer
	app := newCfgAppForTest(&out, filepath.Join(t.TempDir(), "x.json"))

	if err := app.RunConfig([]string{"set", "ai.model"}); err == nil {
		t.Error("set은 KEY VALUE 2개 인자가 필요합니다")
	}
	if err := app.RunConfig([]string{"set"}); err == nil {
		t.Error("set 인자가 없으면 에러가 반환되어야 합니다")
	}
}

func TestRunConfig_ShowInitRejectExtraArgs(t *testing.T) {
	var out bytes.Buffer
	app := newCfgAppForTest(&out, filepath.Join(t.TempDir(), "x.json"))

	if err := app.RunConfig([]string{"show", "extra"}); err == nil {
		t.Error("show는 인자를 받지 않습니다")
	}
	if err := app.RunConfig([]string{"init", "extra"}); err == nil {
		t.Error("init은 인자를 받지 않습니다")
	}
}
