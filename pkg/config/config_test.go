// config_test.go는 poff 설정(로드/검증/적용)의 결정적 단위 테스트입니다.
// 파일 IO는 t.TempDir()로 격리하고, OS 실제 홈 디렉터리에는 의존하지 않습니다.
package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoad_NonexistentFileReturnsEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.json")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("없는 파일 로드는 에러가 아니어야 합니다: %v", err)
	}
	if cfg != (Config{}) {
		t.Errorf("빈 설정이 반환되어야 합니다: %+v", cfg)
	}
}

func TestLoad_InvalidJSONReturnsError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "broken.json")
	if err := os.WriteFile(path, []byte("{invalid"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := Load(path); err == nil {
		t.Fatal("잘못된 JSON은 에러가 반환되어야 합니다")
	}
}

func TestLoad_ValidFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	content := `{
	  "ai": {
	    "model": "llama3.2",
	    "base_url": "http://192.168.1.5:11434/v1",
	    "timeout": "90s"
	  }
	}`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.AI.Model != "llama3.2" {
		t.Errorf("AI.Model = %q, want %q", cfg.AI.Model, "llama3.2")
	}
	if cfg.AI.BaseURL != "http://192.168.1.5:11434/v1" {
		t.Errorf("AI.BaseURL = %q", cfg.AI.BaseURL)
	}
	if time.Duration(cfg.AI.Timeout) != 90*time.Second {
		t.Errorf("AI.Timeout = %v, want 90s", cfg.AI.Timeout)
	}
}

func TestLoad_InvalidTimeoutReturnsError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	content := `{"ai": {"timeout": "not-a-duration"}}`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := Load(path); err == nil {
		t.Fatal("잘못된 timeout 값은 에러가 반환되어야 합니다")
	}
}

// ─── Path ─────────────────────────────────────────────────────────────────────

func TestDefaultPath_JoinsHome(t *testing.T) {
	got := DefaultPath("/home/tester")
	want := filepath.Join("/home/tester", ".poff.json")
	if got != want {
		t.Errorf("DefaultPath() = %q, want %q", got, want)
	}
}

func TestResolvePath_HomeErrorReturnsError(t *testing.T) {
	if _, err := ResolvePath(func() (string, error) { return "", os.ErrNotExist }); err == nil {
		t.Fatal("홈 디렉터리 조회 실패 시 에러가 반환되어야 합니다")
	}
}

// ─── Init ─────────────────────────────────────────────────────────────────────

func TestInit_CreatesDefaultFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".poff.json")

	if err := Init(path); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("생성된 파일을 읽을 수 없습니다: %v", err)
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("생성된 파일이 유효한 JSON이 아닙니다: %v", err)
	}
}

func TestInit_ExistingFileReturnsError(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".poff.json")
	if err := os.WriteFile(path, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := Init(path); err == nil {
		t.Fatal("이미 존재하는 파일에 Init은 에러가 반환되어야 합니다")
	}
}

// ─── Set ──────────────────────────────────────────────────────────────────────

func TestSet_ValidKeys(t *testing.T) {
	tests := []struct {
		name  string
		key   string
		value string
		check func(Config) bool
		want  string
	}{
		{
			name:  "ai.model",
			key:   "ai.model",
			value: "qwen3:8b",
			check: func(c Config) bool { return c.AI.Model == "qwen3:8b" },
		},
		{
			name:  "ai.base_url",
			key:   "ai.base_url",
			value: "http://10.0.0.2:11434/v1",
			check: func(c Config) bool { return c.AI.BaseURL == "http://10.0.0.2:11434/v1" },
		},
		{
			name:  "ai.timeout",
			key:   "ai.timeout",
			value: "2m",
			check: func(c Config) bool { return time.Duration(c.AI.Timeout) == 2*time.Minute },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), ".poff.json")

			if err := Set(path, tt.key, tt.value); err != nil {
				t.Fatalf("Set() error = %v", err)
			}

			cfg, err := Load(path)
			if err != nil {
				t.Fatalf("Set 후 Load 실패: %v", err)
			}
			if !tt.check(cfg) {
				t.Errorf("Set이 반영되지 않았습니다: %+v", cfg)
			}
		})
	}
}

func TestSet_InvalidKeyReturnsError(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".poff.json")

	if err := Set(path, "unknown.key", "x"); err == nil {
		t.Error("알 수 없는 키는 에러가 반환되어야 합니다")
	}
	if err := Set(path, "ai.wrong", "x"); err == nil {
		t.Error("알 수 없는 ai 하위 키는 에러가 반환되어야 합니다")
	}
}

func TestSet_UnknownKeyDoesNotCreateFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".poff.json")

	_ = Set(path, "unknown.key", "x")
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("유효하지 않은 Set은 파일을 생성하지 않아야 합니다")
	}
}

func TestSet_InvalidTimeoutReturnsError(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".poff.json")

	if err := Set(path, "ai.timeout", "abc"); err == nil {
		t.Error("잘못된 timeout은 에러가 반환되어야 합니다")
	}
}

func TestSet_PreservesOtherKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".poff.json")

	if err := Init(path); err != nil {
		t.Fatal(err)
	}
	if err := Set(path, "ai.model", "llama3.2"); err != nil {
		t.Fatal(err)
	}
	// timeout 설정이 사라지면 안 됩니다.
	if err := Set(path, "ai.timeout", "45s"); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AI.Model != "llama3.2" || time.Duration(cfg.AI.Timeout) != 45*time.Second {
		t.Errorf("다른 키가 보존되지 않았습니다: %+v", cfg)
	}
}

// ─── Apply (우선순위 병합 순수 함수) ─────────────────────────────────────────

func TestApply(t *testing.T) {
	tests := []struct {
		name      string
		file      AIConfig
		flags     AIConfig
		wantModel string
		wantURL   string
		wantTO    time.Duration
	}{
		{
			name:      "모두 비었으면 기본값",
			wantModel: DefaultModel,
			wantURL:   DefaultBaseURL,
			wantTO:    DefaultTimeout,
		},
		{
			name:      "파일 값이 기본값을 덮음",
			file:      AIConfig{Model: "file-model"},
			wantModel: "file-model",
			wantURL:   DefaultBaseURL,
			wantTO:    DefaultTimeout,
		},
		{
			name:      "플래그가 파일 값을 덮음",
			file:      AIConfig{Model: "file-model", Timeout: Duration(10 * time.Second)},
			flags:     AIConfig{Model: "flag-model"},
			wantModel: "flag-model",
			wantURL:   DefaultBaseURL,
			wantTO:    10 * time.Second,
		},
		{
			name:      "플래그만 있으면 플래그 사용",
			flags:     AIConfig{BaseURL: "http://flag:1/v1"},
			wantModel: DefaultModel,
			wantURL:   "http://flag:1/v1",
			wantTO:    DefaultTimeout,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Apply(tt.file, tt.flags)
			if got.Model != tt.wantModel {
				t.Errorf("Model = %q, want %q", got.Model, tt.wantModel)
			}
			if got.BaseURL != tt.wantURL {
				t.Errorf("BaseURL = %q, want %q", got.BaseURL, tt.wantURL)
			}
			if time.Duration(got.Timeout) != tt.wantTO {
				t.Errorf("Timeout = %v, want %v", got.Timeout, tt.wantTO)
			}
		})
	}
}
