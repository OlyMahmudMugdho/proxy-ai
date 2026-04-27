package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

// Anthropic types
type AnthropicContent struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

type AnthropicMessage struct {
	Role    string      `json:"role"`
	Content interface{} `json:"content"` // Can be string or []AnthropicContent
}

type AnthropicRequest struct {
	Model       string             `json:"model"`
	Messages    []AnthropicMessage `json:"messages"`
	System      interface{}        `json:"system,omitempty"` // Can be string or []AnthropicContent
	MaxTokens   int                `json:"max_tokens"`
	Stream      bool               `json:"stream,omitempty"`
	Temperature *float64           `json:"temperature,omitempty"`
}

type AnthropicResponse struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Role    string `json:"role"`
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	Model        string `json:"model"`
	StopReason   string `json:"stop_reason"`
	StopSequence string `json:"stop_sequence,omitempty"`
	Usage        struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

// Anthropic Stream Events
type MessageStartEvent struct {
	Type    string `json:"type"`
	Message struct {
		ID    string `json:"id"`
		Type  string `json:"type"`
		Role  string `json:"role"`
		Model string `json:"model"`
		Usage struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	} `json:"message"`
}

type ContentBlockStartEvent struct {
	Type  string `json:"type"`
	Index int    `json:"index"`
	Block struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"block"`
}

type ContentBlockDeltaEvent struct {
	Type  string `json:"type"`
	Index int    `json:"index"`
	Delta struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"delta"`
}

type ContentBlockStopEvent struct {
	Type  string `json:"type"`
	Index int    `json:"index"`
}

type MessageDeltaEvent struct {
	Type  string `json:"type"`
	Delta struct {
		StopReason   string `json:"stop_reason"`
		StopSequence string `json:"stop_sequence,omitempty"`
	} `json:"delta"`
	Usage struct {
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

type MessageStopEvent struct {
	Type string `json:"type"`
}

type PingEvent struct {
	Type string `json:"type"`
}

// OpenAI types
type OpenAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type OpenAIRequest struct {
	Model         string                 `json:"model"`
	Messages      []OpenAIMessage        `json:"messages"`
	MaxTokens     int                    `json:"max_tokens,omitempty"`
	Stream        bool                   `json:"stream,omitempty"`
	StreamOptions *map[string]interface{} `json:"stream_options,omitempty"`
	Temperature   *float64               `json:"temperature,omitempty"`
}

type OpenAIResponse struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	Model   string `json:"model"`
	Choices []struct {
		Index   int `json:"index"`
		Message struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

type OpenAICSDelta struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	Model   string `json:"model"`
	Choices []struct {
		Index int `json:"index"`
		Delta struct {
			Role    string `json:"role,omitempty"`
			Content string `json:"content,omitempty"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

func contentToString(content interface{}) string {
	switch v := content.(type) {
	case string:
		return v
	case []interface{}:
		var parts []string
		for _, part := range v {
			if m, ok := part.(map[string]interface{}); ok {
				if m["type"] == "text" {
					if text, ok := m["text"].(string); ok {
						parts = append(parts, text)
					}
				}
			}
		}
		return strings.Join(parts, "")
	default:
		return ""
	}
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	openaiBaseURL := os.Getenv("OPENAI_BASE_URL")
	if openaiBaseURL == "" {
		openaiBaseURL = "https://api.openai.com/v1"
	}

	openaiAPIKey := os.Getenv("OPENAI_API_KEY")

	http.HandleFunc("/v1/messages/count_tokens", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]int{"input_tokens": 0})
	})

	http.HandleFunc("/v1/messages", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		bodyBytes, _ := io.ReadAll(r.Body)
		// log.Printf("Incoming Anthropic Request: %s", string(bodyBytes))

		var antReq AnthropicRequest
		if err := json.Unmarshal(bodyBytes, &antReq); err != nil {
			log.Printf("Error decoding request: %v", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		log.Printf("Request model: %s, messages: %d, stream: %v", antReq.Model, len(antReq.Messages), antReq.Stream)

		// Translate Anthropic to OpenAI
		oaReq := OpenAIRequest{
			Model:       antReq.Model,
			MaxTokens:   antReq.MaxTokens,
			Stream:      antReq.Stream,
			Temperature: antReq.Temperature,
		}

		if antReq.Stream {
			oaReq.StreamOptions = &map[string]interface{}{"include_usage": true}
		}

		if systemStr := contentToString(antReq.System); systemStr != "" {
			oaReq.Messages = append(oaReq.Messages, OpenAIMessage{
				Role:    "system",
				Content: systemStr,
			})
		}

		for _, msg := range antReq.Messages {
			oaReq.Messages = append(oaReq.Messages, OpenAIMessage{
				Role:    msg.Role,
				Content: contentToString(msg.Content),
			})
		}

		payload, _ := json.Marshal(oaReq)
		req, _ := http.NewRequest(http.MethodPost, openaiBaseURL+"/chat/completions", bytes.NewBuffer(payload))
		req.Header.Set("Content-Type", "application/json")
		
		// Set Authorization header
		// Priority: 1. Environment Variable (OPENAI_API_KEY) 2. Incoming Authorization 3. Incoming X-Api-Key
		finalToken := openaiAPIKey
		if finalToken == "" {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				authHeader = r.Header.Get("X-Api-Key")
			}
			finalToken = strings.TrimPrefix(authHeader, "Bearer ")
		}

		if finalToken != "" {
			req.Header.Set("Authorization", "Bearer "+finalToken)
		}

		client := &http.Client{
			Timeout: 2 * time.Minute,
		}
		resp, err := client.Do(req)
		if err != nil {
			log.Printf("Error calling backend: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			log.Printf("Backend returned error %d: %s", resp.StatusCode, string(body))
			w.WriteHeader(resp.StatusCode)
			w.Write(body)
			return
		}

		if antReq.Stream {
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Cache-Control", "no-cache")
			w.Header().Set("Connection", "keep-alive")

			flusher, ok := w.(http.Flusher)
			if !ok {
				http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
				return
			}

			// Send an initial ping or wait a bit? 
			// Anthropic usually starts with message_start quickly.

			scanner := bufio.NewScanner(resp.Body)
			first := true
			id := "msg_" + time.Now().Format("20060102150405")
			model := antReq.Model

			for scanner.Scan() {
				line := scanner.Text()
				if !strings.HasPrefix(line, "data: ") {
					continue
				}
				data := strings.TrimPrefix(line, "data: ")
				if data == "[DONE]" {
					break
				}

				var oaDelta OpenAICSDelta
				if err := json.Unmarshal([]byte(data), &oaDelta); err != nil {
					continue
				}

				if first {
					if oaDelta.ID != "" {
						id = oaDelta.ID
					}
					if oaDelta.Model != "" {
						model = oaDelta.Model
					}
					// Send message_start
					var startEv MessageStartEvent
					startEv.Type = "message_start"
					startEv.Message.ID = id
					startEv.Message.Type = "message"
					startEv.Message.Role = "assistant"
					startEv.Message.Model = model
					sendEvent(w, "message_start", startEv)

					// Send content_block_start
					var cbStart ContentBlockStartEvent
					cbStart.Type = "content_block_start"
					cbStart.Index = 0
					cbStart.Block.Type = "text"
					cbStart.Block.Text = ""
					sendEvent(w, "content_block_start", cbStart)

					first = false
				}

				if len(oaDelta.Choices) > 0 {
					content := oaDelta.Choices[0].Delta.Content
					if content != "" {
						var cbDelta ContentBlockDeltaEvent
						cbDelta.Type = "content_block_delta"
						cbDelta.Index = 0
						cbDelta.Delta.Type = "text_delta"
						cbDelta.Delta.Text = content
						sendEvent(w, "content_block_delta", cbDelta)
					}

					if oaDelta.Choices[0].FinishReason != "" {
						// Send content_block_stop
						var cbStop ContentBlockStopEvent
						cbStop.Type = "content_block_stop"
						cbStop.Index = 0
						sendEvent(w, "content_block_stop", cbStop)

						// Send message_delta
						var mDelta MessageDeltaEvent
						mDelta.Type = "message_delta"
						mDelta.Delta.StopReason = oaDelta.Choices[0].FinishReason
						if mDelta.Delta.StopReason == "stop" {
							mDelta.Delta.StopReason = "end_turn"
						}
						sendEvent(w, "message_delta", mDelta)

						// Send message_stop
						var mStop MessageStopEvent
						mStop.Type = "message_stop"
						sendEvent(w, "message_stop", mStop)
					}
				}
				flusher.Flush()
			}
			return
		}

		var oaResp OpenAIResponse
		if err := json.NewDecoder(resp.Body).Decode(&oaResp); err != nil {
			log.Printf("Error decoding backend response: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Translate OpenAI to Anthropic
		antResp := AnthropicResponse{
			ID:    oaResp.ID,
			Type:  "message",
			Role:  "assistant",
			Model: oaResp.Model,
		}

		if len(oaResp.Choices) > 0 {
			antResp.Content = append(antResp.Content, struct {
				Type string `json:"type"`
				Text string `json:"text"`
			}{
				Type: "text",
				Text: oaResp.Choices[0].Message.Content,
			})
			antResp.StopReason = oaResp.Choices[0].FinishReason
			if antResp.StopReason == "stop" {
				antResp.StopReason = "end_turn"
			}
		}

		antResp.Usage.InputTokens = oaResp.Usage.PromptTokens
		antResp.Usage.OutputTokens = oaResp.Usage.CompletionTokens

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(antResp)
	})

	log.Printf("Starting proxy on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

func sendEvent(w io.Writer, event string, data interface{}) {
	b, _ := json.Marshal(data)
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, string(b))
}
