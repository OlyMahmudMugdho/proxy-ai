package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"proxy-ai/internal/config"
	"proxy-ai/internal/proxy"
	"strings"
)

func main() {
	cfg := config.Load()

	profileFlag := flag.String("profile", "", "The profile to use (for launcher mode)")
	verboseFlag := flag.Bool("verbose", false, "Show proxy logs in terminal")
	flag.Parse()

	args := flag.Args()

	// 1. SERVE MODE
	if len(args) > 0 && args[0] == "serve" {
		http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			if strings.HasSuffix(r.URL.Path, "/v1/messages") {
				proxy.HandleMessages(cfg, w, r)
			} else if strings.HasSuffix(r.URL.Path, "/v1/messages/count_tokens") {
				json.NewEncoder(w).Encode(map[string]int{"input_tokens": 0})
			} else {
				http.NotFound(w, r)
			}
		})

		log.Printf("Proxy-AI Server listening on :%s", cfg.Port)
		log.Printf("Default profile: %s", cfg.DefaultProfile)
		log.Fatal(http.ListenAndServe(":"+cfg.Port, nil))
		return
	}

	// 2. LAUNCHER MODE
	targetProfile := *profileFlag
	if targetProfile == "" {
		targetProfile = cfg.DefaultProfile
	}

	p, ok := cfg.Profiles[targetProfile]
	if !ok {
		log.Fatalf("Profile '%s' not found in %s", targetProfile, config.GetConfigPath())
	}

	// Redirect logs to file if not verbose
	if !*verboseFlag {
		f, err := os.OpenFile(config.GetLogPath(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err == nil {
			log.SetOutput(f)
			defer f.Close()
		}
	}

	fmt.Printf("🚀 Launching Claude Code [%s]...\n", targetProfile)
	fmt.Printf("📝 Logs available at: %s\n", config.GetLogPath())

	cmd := exec.Command("claude")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	baseURL := fmt.Sprintf("http://localhost:%s/%s", cfg.Port, targetProfile)

	defaultModel := "claude-3-7-sonnet-20250219"
	for k := range p.ModelMapping {
		defaultModel = k
		break
	}

	cmd.Env = append(os.Environ(),
		"ANTHROPIC_BASE_URL="+baseURL,
		"ANTHROPIC_API_KEY=dummy",
		"ANTHROPIC_MODEL="+defaultModel,
	)

	err := cmd.Run()
	if err != nil {
		fmt.Printf("\nClaude exited: %v\n", err)
	}
}
