package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// --- Configuration ---

type Profile struct {
	OpenAIBaseURL string            `yaml:"openai_base_url"`
	OpenAIAPIKey  string            `yaml:"openai_api_key"`
	ModelMapping  map[string]string `yaml:"model_mapping"`
}

type Config struct {
	Port           string             `yaml:"port"`
	Profiles       map[string]Profile `yaml:"profiles"`
	DefaultProfile string             `yaml:"default_profile"`
}

func getHomeDir() string {
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, ".proxy-ai")
	os.MkdirAll(dir, 0755)
	return dir
}

func getConfigPath() string {
	return filepath.Join(getHomeDir(), "config.yaml")
}

func loadConfig() *Config {
	cfg := &Config{
		Port:     "8080",
		Profiles: make(map[string]Profile),
	}
	data, err := os.ReadFile(getConfigPath())
	if err == nil {
		yaml.Unmarshal(data, cfg)
	}
	return cfg
}

// --- Proxy Logic ---

func handleMessages(cfg *Config, w http.ResponseWriter, r *http.Request) {
	// Determine Profile from URL path: /<profile>/v1/messages or just /v1/messages
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	profileName := cfg.DefaultProfile
	if len(parts) > 2 && parts[len(parts)-2] == "v1" {
		// Path like /opencode/v1/messages
		profileName = parts[0]
	}

	p, ok := cfg.Profiles[profileName]
	if !ok {
		log.Printf("Error: Profile '%s' not found", profileName)
		http.Error(w, fmt.Sprintf("Profile '%s' not found", profileName), 404)
		return
	}

	bodyBytes, _ := io.ReadAll(r.Body)
	var antReq AnthropicRequest
	json.Unmarshal(bodyBytes, &antReq)

	targetModel := antReq.Model
	if mapped, ok := p.ModelMapping[antReq.Model]; ok {
		targetModel = mapped
	}

	log.Printf("[%s] %s -> %s", profileName, antReq.Model, targetModel)

	oaReq := OpenAIRequest{
		Model: targetModel, MaxTokens: antReq.MaxTokens, Stream: antReq.Stream, Temperature: antReq.Temperature,
	}
	if s := contentToString(antReq.System); s != "" {
		oaReq.Messages = append(oaReq.Messages, OpenAIMessage{Role: "system", Content: s})
	}
	oaReq.Messages = append(oaReq.Messages, translateMessages(antReq.Messages)...)

	for _, t := range antReq.Tools {
		ot := OpenAITool{Type: "function"}
		ot.Function.Name = t.Name; ot.Function.Description = t.Description; ot.Function.Parameters = t.InputSchema
		oaReq.Tools = append(oaReq.Tools, ot)
	}

	payload, _ := json.Marshal(oaReq)
	req, _ := http.NewRequest(http.MethodPost, p.OpenAIBaseURL+"/chat/completions", bytes.NewBuffer(payload))
	req.Header.Set("Content-Type", "application/json")

	token := p.OpenAIAPIKey
	if token == "" {
		token = strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if token == "" { token = r.Header.Get("X-Api-Key") }
	}
	if token != "" { req.Header.Set("Authorization", "Bearer "+token) }

	resp, err := (&http.Client{Timeout: 5 * time.Minute}).Do(req)
	if err != nil { 
		log.Printf("[%s] Backend Error: %v", profileName, err)
		http.Error(w, err.Error(), 500)
		return 
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("[%s] Backend returned error %d: %s", profileName, resp.StatusCode, string(body))
		w.WriteHeader(resp.StatusCode)
		w.Write(body)
		return
	}

	if antReq.Stream {
		handleStream(w, resp, antReq.Model)
		return
	}

	var oaResp OpenAIResponse
	json.NewDecoder(resp.Body).Decode(&oaResp)
	antResp := AnthropicResponse{ID: oaResp.ID, Type: "message", Role: "assistant", Model: antReq.Model}
	if len(oaResp.Choices) > 0 {
		c := oaResp.Choices[0].Message
		if c.Content != "" { antResp.Content = append(antResp.Content, AnthropicContent{Type: "text", Text: c.Content}) }
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(antResp)
}

func main() {
	cfg := loadConfig()

	profileFlag := flag.String("profile", "", "The profile to use (for launcher mode)")
	flag.Parse()

	args := flag.Args()

	// 1. SERVE MODE
	if len(args) > 0 && args[0] == "serve" {
		http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			if strings.HasSuffix(r.URL.Path, "/v1/messages") {
				handleMessages(cfg, w, r)
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
		log.Fatalf("Profile '%s' not found in ~/.proxy-ai/config.yaml", targetProfile)
	}

	fmt.Printf("🚀 Launching Claude Code with profile: %s\n", targetProfile)

	cmd := exec.Command("claude")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	// The Magic: Point to a sub-URL for this specific profile
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

// --- Translation Logic ---

func handleStream(w http.ResponseWriter, resp *http.Response, requestedModel string) {
	w.Header().Set("Content-Type", "text/event-stream")
	flusher := w.(http.Flusher)
	scanner := bufio.NewScanner(resp.Body)
	
	first := true
	msgID := "msg_" + time.Now().Format("20060102150405")
	nextIdx := 0
	thinkingIdx, textIdx, toolIdx := -1, -1, -1
	activeToolID := ""
	finalStopReason := "end_turn"
	var finalUsage *OpenAIUsage

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") { continue }
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" { break }

		var oaDelta OpenAICSDelta
		if err := json.Unmarshal([]byte(data), &oaDelta); err != nil { continue }

		if first {
			if oaDelta.ID != "" { msgID = "msg_" + oaDelta.ID }
			sendEvent(w, "message_start", map[string]interface{}{
				"type": "message_start",
				"message": map[string]interface{}{
					"id": msgID, "type": "message", "role": "assistant", "model": requestedModel,
				},
			})
			first = false
		}
		if fr := findFinishReason(oaDelta); fr != "" {
			finalStopReason = fr
			if finalStopReason == "stop" { finalStopReason = "end_turn" }
		}
		if oaDelta.Usage != nil { finalUsage = oaDelta.Usage }

		if len(oaDelta.Choices) > 0 {
			d := oaDelta.Choices[0].Delta
			
			// 1. Reasoning
			rStr := d.Reasoning
			if rStr == "" { rStr = d.ReasoningContent }
			if rStr != "" {
				if thinkingIdx == -1 {
					thinkingIdx = nextIdx; nextIdx++
					sendEvent(w, "content_block_start", map[string]interface{}{
						"type": "content_block_start", "index": thinkingIdx,
						"content_block": map[string]string{"type": "thinking", "thinking": ""},
					})
				}
				sendEvent(w, "content_block_delta", map[string]interface{}{
					"type": "content_block_delta", "index": thinkingIdx,
					"delta": map[string]string{"type": "thinking_delta", "thinking": rStr},
				})
			}

			// 2. Tool Calls
			if len(d.ToolCalls) > 0 {
				tc := d.ToolCalls[0]
				if tc.ID != "" {
					if thinkingIdx != -1 {
						sendEvent(w, "content_block_stop", map[string]interface{}{"type": "content_block_stop", "index": thinkingIdx})
						thinkingIdx = -1
					}
					if textIdx != -1 {
						sendEvent(w, "content_block_stop", map[string]interface{}{"type": "content_block_stop", "index": textIdx})
						textIdx = -1
					}
					activeToolID = tc.ID
					toolIdx = nextIdx; nextIdx++
					sendEvent(w, "content_block_start", map[string]interface{}{
						"type": "content_block_start", "index": toolIdx,
						"content_block": map[string]interface{}{
							"type": "tool_use", "id": activeToolID, "name": tc.Function.Name, "input": map[string]interface{}{},
						},
					})
				}
				if tc.Function.Arguments != "" {
					sendEvent(w, "content_block_delta", map[string]interface{}{
						"type": "content_block_delta", "index": toolIdx,
						"delta": map[string]interface{}{"type": "input_json_delta", "partial_json": tc.Function.Arguments},
					})
				}
			}

			// 3. Text
			if d.Content != "" {
				if thinkingIdx != -1 {
					sendEvent(w, "content_block_stop", map[string]interface{}{"type": "content_block_stop", "index": thinkingIdx})
					thinkingIdx = -1
				}
				if textIdx == -1 {
					textIdx = nextIdx; nextIdx++
					sendEvent(w, "content_block_start", map[string]interface{}{
						"type": "content_block_start", "index": textIdx,
						"content_block": map[string]string{"type": "text", "text": ""},
					})
				}
				sendEvent(w, "content_block_delta", map[string]interface{}{
					"type": "content_block_delta", "index": textIdx,
					"delta": map[string]interface{}{"type": "text_delta", "text": d.Content},
				})
			}
		}
		flusher.Flush()
	}

	if thinkingIdx != -1 { sendEvent(w, "content_block_stop", map[string]interface{}{"type": "content_block_stop", "index": thinkingIdx}) }
	if textIdx != -1 { sendEvent(w, "content_block_stop", map[string]interface{}{"type": "content_block_stop", "index": textIdx}) }
	if toolIdx != -1 { sendEvent(w, "content_block_stop", map[string]interface{}{"type": "content_block_stop", "index": toolIdx}) }
	
	if activeToolID != "" { finalStopReason = "tool_use" }
	mDelta := map[string]interface{}{
		"type": "message_delta",
		"delta": map[string]interface{}{"stop_reason": finalStopReason, "stop_sequence": nil},
	}
	if finalUsage != nil { mDelta["usage"] = map[string]interface{}{"output_tokens": finalUsage.CompletionTokens} }
	sendEvent(w, "message_delta", mDelta)
	sendEvent(w, "message_stop", map[string]interface{}{"type": "message_stop"})
	flusher.Flush()
}

func findFinishReason(delta OpenAICSDelta) string {
	if len(delta.Choices) > 0 {
		return delta.Choices[0].FinishReason
	}
	return ""
}

func sendEvent(w io.Writer, event string, data interface{}) {
	b, _ := json.Marshal(data)
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, string(b))
}

func stringify(v interface{}) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func contentToString(content interface{}) string {
	switch v := content.(type) {
	case string: return v
	case []interface{}:
		var parts []string
		for _, part := range v {
			if m, ok := part.(map[string]interface{}); ok {
				if t, ok := m["type"].(string); ok && t == "text" {
					if text, ok := m["text"].(string); ok { parts = append(parts, text) }
				}
			}
		}
		return strings.Join(parts, "")
	default: return ""
	}
}

func translateMessages(antMsgs []AnthropicMessage) []OpenAIMessage {
	var oaMsgs []OpenAIMessage
	for _, m := range antMsgs {
		oaMsg := OpenAIMessage{Role: m.Role}
		switch v := m.Content.(type) {
		case string:
			oaMsg.Content = v
		case []interface{}:
			var textParts []string
			for _, part := range v {
				if mPart, ok := part.(map[string]interface{}); ok {
					pType, _ := mPart["type"].(string)
					switch pType {
					case "text":
						textParts = append(textParts, mPart["text"].(string))
					case "tool_use":
						oaMsg.ToolCalls = append(oaMsg.ToolCalls, OpenAIToolCall{
							ID:   mPart["id"].(string),
							Type: "function",
							Function: struct {
								Name      string `json:"name"`
								Arguments string `json:"arguments"`
							}{
								Name:      mPart["name"].(string),
								Arguments: stringify(mPart["input"]),
							},
						})
					case "tool_result":
						oaMsg.Role = "tool"
						oaMsg.ToolCallID = mPart["tool_use_id"].(string)
						oaMsg.Content = mPart["content"]
					}
				}
			}
			if len(textParts) > 0 { oaMsg.Content = strings.Join(textParts, "") }
		}
		oaMsgs = append(oaMsgs, oaMsg)
	}
	return oaMsgs
}

// --- Types ---

type AnthropicContent struct {
	Type      string      `json:"type"`
	Text      string      `json:"text,omitempty"`
	Thinking  string      `json:"thinking,omitempty"`
	ID        string      `json:"id,omitempty"`
	Name      string      `json:"name,omitempty"`
	Input     interface{} `json:"input,omitempty"`
	ToolUseID string      `json:"tool_use_id,omitempty"`
	Content   interface{} `json:"content,omitempty"`
}

type AnthropicMessage struct {
	Role    string      `json:"role"`
	Content interface{} `json:"content"`
}

type AnthropicTool struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	InputSchema interface{} `json:"input_schema"`
}

type AnthropicRequest struct {
	Model       string             `json:"model"`
	Messages    []AnthropicMessage `json:"messages"`
	System      interface{}        `json:"system,omitempty"`
	MaxTokens   int                `json:"max_tokens"`
	Stream      bool               `json:"stream,omitempty"`
	Temperature *float64           `json:"temperature,omitempty"`
	Tools       []AnthropicTool    `json:"tools,omitempty"`
}

type AnthropicResponse struct {
	ID      string             `json:"id"`
	Type    string             `json:"type"`
	Role    string             `json:"role"`
	Content []AnthropicContent `json:"content"`
	Model   string             `json:"model"`
	Usage   struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
	StopReason string `json:"stop_reason"`
}

type OpenAIMessage struct {
	Role       string           `json:"role"`
	Content    interface{}      `json:"content"`
	ToolCalls  []OpenAIToolCall `json:"tool_calls,omitempty"`
	ToolCallID string           `json:"tool_call_id,omitempty"`
}

type OpenAIToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type OpenAITool struct {
	Type     string `json:"type"`
	Function struct {
		Name        string      `json:"name"`
		Description string      `json:"description"`
		Parameters  interface{} `json:"parameters"`
	} `json:"function"`
}

type OpenAIRequest struct {
	Model       string          `json:"model"`
	Messages    []OpenAIMessage `json:"messages"`
	MaxTokens   int             `json:"max_tokens,omitempty"`
	Stream      bool            `json:"stream,omitempty"`
	Temperature *float64        `json:"temperature,omitempty"`
	Tools       []OpenAITool    `json:"tools,omitempty"`
}

type OpenAIUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
}

type OpenAIResponse struct {
	ID      string `json:"id"`
	Model   string `json:"model"`
	Choices []struct {
		Message struct {
			Role             string           `json:"role"`
			Content          string           `json:"content"`
			ToolCalls        []OpenAIToolCall `json:"tool_calls"`
			Reasoning        string           `json:"reasoning,omitempty"`
			ReasoningContent string           `json:"reasoning_content,omitempty"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage OpenAIUsage `json:"usage"`
}

type OpenAICSDelta struct {
	ID      string `json:"id"`
	Choices []struct {
		Delta struct {
			Content          string           `json:"content,omitempty"`
			Reasoning        string           `json:"reasoning,omitempty"`
			ReasoningContent string           `json:"reasoning_content,omitempty"`
			ToolCalls        []OpenAIToolCall `json:"tool_calls,omitempty"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage *OpenAIUsage `json:"usage,omitempty"`
}
