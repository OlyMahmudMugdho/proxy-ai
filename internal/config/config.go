package config

import (
	"log"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Profile struct {
	OpenAIBaseURL   string            `yaml:"openai_base_url"`
	OpenAIAPIKey    string            `yaml:"openai_api_key"`
	OpenAIAPIKeyEnv string            `yaml:"openai_api_key_env"`
	ModelMapping    map[string]string `yaml:"model_mapping"`
}

func (p *Profile) GetAPIKey() string {
	if p.OpenAIAPIKey != "" {
		return p.OpenAIAPIKey
	}
	if p.OpenAIAPIKeyEnv != "" {
		return os.Getenv(p.OpenAIAPIKeyEnv)
	}
	return ""
}

type Config struct {
	Port           string             `yaml:"port"`
	Profiles       map[string]Profile `yaml:"profiles"`
	DefaultProfile string             `yaml:"default_profile"`
}

func GetHomeDir() string {
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, ".proxy-ai")
	os.MkdirAll(dir, 0755)
	return dir
}

func GetConfigPath() string {
	return filepath.Join(GetHomeDir(), "config.yaml")
}

func GetLogPath() string {
	return filepath.Join(GetHomeDir(), "proxy.log")
}

func Load() *Config {
	cfg := &Config{
		Port:     "8080",
		Profiles: make(map[string]Profile),
	}

	data, err := os.ReadFile(GetConfigPath())
	if err == nil {
		yaml.Unmarshal(data, cfg)
	} else if os.IsNotExist(err) {
		cfg.DefaultProfile = "opencode"
		cfg.Profiles["opencode"] = Profile{
			OpenAIBaseURL: "https://opencode.ai/zen/v1",
			OpenAIAPIKey:  "your-key-here",
			ModelMapping: map[string]string{
				"claude-opus-4-7":   "minimax-m2.5-free",
				"claude-sonnet-4-6": "big-pickle",
			},
		}
		Save(cfg)
		log.Printf("Created default config at %s", GetConfigPath())
	}

	return cfg
}

func Save(cfg *Config) {
	data, _ := yaml.Marshal(cfg)
	os.WriteFile(GetConfigPath(), data, 0644)
}
