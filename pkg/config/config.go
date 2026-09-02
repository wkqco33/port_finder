// Package config는 poff의 설정 파일(~/.poff.json) 로드/초기화/변경 기능을 제공합니다.
// 파일 IO와 검증 로직을 분리해 단위 테스트가 쉽고, cmd 계층은 이 API만 사용합니다.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// 기본값은 pkg/ai와 중복 정의하지 않고 여기서 단일 출처로 관리합니다.
const (
	// DefaultModel은 AI 분석 기본 Ollama 모델입니다.
	DefaultModel = "qwen3:4b"
	// DefaultBaseURL은 기본 Ollama 엔드포인트입니다.
	DefaultBaseURL = "http://localhost:11434/v1"
	// DefaultTimeout은 AI 요청 기본 타임아웃입니다.
	DefaultTimeout = 60 * time.Second
	// FileName은 설정 파일명입니다 (~/.poff.json).
	FileName = ".poff.json"
)

// Duration은 JSON 문자열("90s" 등)과 time.Duration을 상호 변환하는 설정용 타입입니다.
type Duration time.Duration

// UnmarshalJSON은 문자열 또는 숫자(나노초) 입력을 파싱합니다.
func (d *Duration) UnmarshalJSON(data []byte) error {
	s := strings.Trim(string(data), `"`)
	if s == "" || s == "null" {
		return nil
	}
	v, err := time.ParseDuration(s)
	if err != nil {
		return fmt.Errorf("유효하지 않은 duration 값 %q (예: 90s, 2m): %w", s, err)
	}
	*d = Duration(v)
	return nil
}

// MarshalJSON은 duration을 사람이 읽는 문자열로 저장합니다.
func (d Duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(time.Duration(d).String())
}

// AIConfig는 AI 분석 관련 설정입니다.
// 제로 값은 "설정되지 않음"을 의미하며 Apply에서 기본값으로 채워집니다.
type AIConfig struct {
	Model   string   `json:"model,omitempty"`
	BaseURL string   `json:"base_url,omitempty"`
	Timeout Duration `json:"timeout,omitempty"`
}

// Config는 poff 설정 파일의 전체 구조입니다.
type Config struct {
	AI AIConfig `json:"ai"`
}

// DefaultPath는 홈 디렉터리 기준 설정 파일 경로를 반환합니다.
func DefaultPath(home string) string {
	return filepath.Join(home, FileName)
}

// ResolvePath는 실제 홈 디렉터리를 조회해 설정 파일 경로를 반환합니다.
func ResolvePath(homeDir func() (string, error)) (string, error) {
	home, err := homeDir()
	if err != nil {
		return "", fmt.Errorf("홈 디렉터리를 찾을 수 없습니다: %w", err)
	}
	return DefaultPath(home), nil
}

// Load는 설정 파일을 읽어 Config를 반환합니다.
// 파일이 없으면 (에러가 아니라) 빈 설정을 반환합니다 — 기본값 동작을 방해하지 않기 위함입니다.
func Load(path string) (Config, error) {
	var cfg Config

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Config{}, nil
		}
		return Config{}, fmt.Errorf("설정 파일을 읽을 수 없습니다: %w", err)
	}

	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("설정 파일 파싱 실패 (%s): %w", path, err)
	}

	// timeout이 문자열/잘못된 값이면 여기서 걸러냅니다 (0 또는 음수는 미설정으로 처리).
	if cfg.AI.Timeout < 0 {
		return Config{}, fmt.Errorf("설정 파일 파싱 실패: ai.timeout은 양수여야 합니다")
	}
	return cfg, nil
}

// Init는 기본값이 채워진 설정 파일을 새로 생성합니다.
// 이미 파일이 있으면 덮어쓰지 않고 에러를 반환합니다.
func Init(path string) error {
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("설정 파일이 이미 존재합니다: %s", path)
	}

	cfg := Config{AI: AIConfig{
		Model:   DefaultModel,
		BaseURL: DefaultBaseURL,
		Timeout: Duration(DefaultTimeout),
	}}
	return write(cfg, path)
}

// Set은 설정 파일의 단일 키를 변경합니다.
// 파일이 없으면 기본값 구조로 새로 생성합니다.
func Set(path, key, value string) error {
	cfg, err := Load(path)
	if err != nil {
		return err
	}

	switch key {
	case "ai.model":
		cfg.AI.Model = value
	case "ai.base_url":
		cfg.AI.BaseURL = value
	case "ai.timeout":
		d, err := time.ParseDuration(value)
		if err != nil {
			return fmt.Errorf("유효하지 않은 duration 값 %q (예: 90s, 2m): %w", value, err)
		}
		if d <= 0 {
			return fmt.Errorf("ai.timeout은 양수여야 합니다: %v", d)
		}
		cfg.AI.Timeout = Duration(d)
	default:
		return fmt.Errorf("알 수 없는 설정 키: %q (사용 가능: ai.model, ai.base_url, ai.timeout)", key)
	}

	return write(cfg, path)
}

// Apply는 설정 우선순위(플래그 > 파일 > 기본값)에 따라 최종 AI 설정을 만드는 순수 함수입니다.
// 제로 값은 "설정되지 않음"으로 간주합니다.
func Apply(file, flags AIConfig) AIConfig {
	out := AIConfig{
		Model:   DefaultModel,
		BaseURL: DefaultBaseURL,
		Timeout: Duration(DefaultTimeout),
	}
	// 파일 값이 기본값을 덮고, 플래그가 파일 값을 덮습니다.
	for _, src := range []AIConfig{file, flags} {
		if src.Model != "" {
			out.Model = src.Model
		}
		if src.BaseURL != "" {
			out.BaseURL = src.BaseURL
		}
		if src.Timeout > 0 {
			out.Timeout = src.Timeout
		}
	}
	return out
}

// write는 Config를 사람이 읽기 좋은 JSON(2-space indent)으로 저장합니다.
func write(cfg Config, path string) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("설정 직렬화 실패: %w", err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		return fmt.Errorf("설정 파일 저장 실패 (%s): %w", path, err)
	}
	return nil
}
