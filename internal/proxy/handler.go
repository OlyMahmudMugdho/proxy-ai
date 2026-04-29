package proxy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"proxy-ai/internal/config"
	"proxy-ai/internal/translator"
	"proxy-ai/internal/types"
	"strings"
	"time"
)

func HandleMessages(cfg *config.Config, w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	profileName := cfg.DefaultProfile
	if len(parts) > 2 && parts[len(parts)-2] == "v1" {
		profileName = parts[0]
	}

	p, ok := cfg.Profiles[profileName]
	if !ok {
		log.Printf("Error: Profile '%s' not found", profileName)
		http.Error(w, fmt.Sprintf("Profile '%s' not found", profileName), 404)
		return
	}

	bodyBytes, _ := io.ReadAll(r.Body)
	var antReq types.AnthropicRequest
	json.Unmarshal(bodyBytes, &antReq)

	targetModel := antReq.Model
	if mapped, ok := p.ModelMapping[antReq.Model]; ok {
		targetModel = mapped
	}

	log.Printf("[%s] %s -> %s", profileName, antReq.Model, targetModel)

	oaReq := types.OpenAIRequest{
		Model:       targetModel,
		MaxTokens:   antReq.MaxTokens,
		Stream:      antReq.Stream,
		Temperature: antReq.Temperature,
	}
	if s := translator.ContentToString(antReq.System); s != "" {
		oaReq.Messages = append(oaReq.Messages, types.OpenAIMessage{Role: "system", Content: s})
	}
	oaReq.Messages = append(oaReq.Messages, translator.TranslateMessages(antReq.Messages)...)

	for _, t := range antReq.Tools {
		ot := types.OpenAITool{Type: "function"}
		ot.Function.Name = t.Name
		ot.Function.Description = t.Description
		ot.Function.Parameters = t.InputSchema
		oaReq.Tools = append(oaReq.Tools, ot)
	}

	payload, _ := json.Marshal(oaReq)
	req, _ := http.NewRequest(http.MethodPost, p.OpenAIBaseURL+"/chat/completions", bytes.NewBuffer(payload))
	req.Header.Set("Content-Type", "application/json")

	token := p.GetAPIKey()
	if token == "" {
		token = strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if token == "" {
			token = r.Header.Get("X-Api-Key")
		}
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

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
		HandleStream(w, resp, antReq.Model)
		return
	}

	var oaResp types.OpenAIResponse
	json.NewDecoder(resp.Body).Decode(&oaResp)
	antResp := types.AnthropicResponse{
		ID:    oaResp.ID,
		Type:  "message",
		Role:  "assistant",
		Model: antReq.Model,
	}
	if len(oaResp.Choices) > 0 {
		choice := oaResp.Choices[0].Message
		if choice.Content != "" {
			antResp.Content = append(antResp.Content, types.AnthropicContent{Type: "text", Text: choice.Content})
		}
	}
	antResp.Usage.InputTokens = oaResp.Usage.PromptTokens
	antResp.Usage.OutputTokens = oaResp.Usage.CompletionTokens

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(antResp)
}
