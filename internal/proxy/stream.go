package proxy

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"proxy-ai/internal/types"
	"strings"
	"time"
)

func HandleStream(w http.ResponseWriter, resp *http.Response, requestedModel string) {
	w.Header().Set("Content-Type", "text/event-stream")
	flusher := w.(http.Flusher)
	scanner := bufio.NewScanner(resp.Body)

	first := true
	msgID := "msg_" + time.Now().Format("20060102150405")
	nextIdx := 0
	thinkingIdx, textIdx, toolIdx := -1, -1, -1
	activeToolID := ""
	finalStopReason := "end_turn"
	var finalUsage *types.OpenAIUsage

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			break
		}

		var oaDelta types.OpenAICSDelta
		if err := json.Unmarshal([]byte(data), &oaDelta); err != nil {
			continue
		}

		if first {
			if oaDelta.ID != "" {
				msgID = "msg_" + oaDelta.ID
			}
			SendEvent(w, "message_start", map[string]interface{}{
				"type": "message_start",
				"message": map[string]interface{}{
					"id": msgID, "type": "message", "role": "assistant", "model": requestedModel,
				},
			})
			first = false
		}
		if fr := findFinishReason(oaDelta); fr != "" {
			finalStopReason = fr
			if finalStopReason == "stop" {
				finalStopReason = "end_turn"
			}
		}
		if oaDelta.Usage != nil {
			finalUsage = oaDelta.Usage
		}

		if len(oaDelta.Choices) > 0 {
			d := oaDelta.Choices[0].Delta

			// 1. Reasoning
			rStr := d.Reasoning
			if rStr == "" {
				rStr = d.ReasoningContent
			}
			if rStr != "" {
				if thinkingIdx == -1 {
					thinkingIdx = nextIdx
					nextIdx++
					SendEvent(w, "content_block_start", map[string]interface{}{
						"type": "content_block_start", "index": thinkingIdx,
						"content_block": map[string]string{"type": "thinking", "thinking": ""},
					})
				}
				SendEvent(w, "content_block_delta", map[string]interface{}{
					"type": "content_block_delta", "index": thinkingIdx,
					"delta": map[string]string{"type": "thinking_delta", "thinking": rStr},
				})
			}

			// 2. Tool Calls
			if len(d.ToolCalls) > 0 {
				tc := d.ToolCalls[0]
				if tc.ID != "" {
					if thinkingIdx != -1 {
						SendEvent(w, "content_block_stop", map[string]interface{}{"type": "content_block_stop", "index": thinkingIdx})
						thinkingIdx = -1
					}
					if textIdx != -1 {
						SendEvent(w, "content_block_stop", map[string]interface{}{"type": "content_block_stop", "index": textIdx})
						textIdx = -1
					}
					activeToolID = tc.ID
					toolIdx = nextIdx
					nextIdx++
					SendEvent(w, "content_block_start", map[string]interface{}{
						"type": "content_block_start", "index": toolIdx,
						"content_block": map[string]interface{}{
							"type": "tool_use", "id": activeToolID, "name": tc.Function.Name, "input": map[string]interface{}{},
						},
					})
				}
				if tc.Function.Arguments != "" {
					SendEvent(w, "content_block_delta", map[string]interface{}{
						"type": "content_block_delta", "index": toolIdx,
						"delta": map[string]interface{}{"type": "input_json_delta", "partial_json": tc.Function.Arguments},
					})
				}
			}

			// 3. Text
			if d.Content != "" {
				if thinkingIdx != -1 {
					SendEvent(w, "content_block_stop", map[string]interface{}{"type": "content_block_stop", "index": thinkingIdx})
					thinkingIdx = -1
				}
				if textIdx == -1 {
					textIdx = nextIdx
					nextIdx++
					SendEvent(w, "content_block_start", map[string]interface{}{
						"type": "content_block_start", "index": textIdx,
						"content_block": map[string]string{"type": "text", "text": ""},
					})
				}
				SendEvent(w, "content_block_delta", map[string]interface{}{
					"type": "content_block_delta", "index": textIdx,
					"delta": map[string]interface{}{"type": "text_delta", "text": d.Content},
				})
			}
		}
		flusher.Flush()
	}

	if thinkingIdx != -1 {
		SendEvent(w, "content_block_stop", map[string]interface{}{"type": "content_block_stop", "index": thinkingIdx})
	}
	if textIdx != -1 {
		SendEvent(w, "content_block_stop", map[string]interface{}{"type": "content_block_stop", "index": textIdx})
	}
	if toolIdx != -1 {
		SendEvent(w, "content_block_stop", map[string]interface{}{"type": "content_block_stop", "index": toolIdx})
	}

	if activeToolID != "" {
		finalStopReason = "tool_use"
	}
	mDelta := map[string]interface{}{
		"type": "message_delta",
		"delta": map[string]interface{}{"stop_reason": finalStopReason, "stop_sequence": nil},
	}
	if finalUsage != nil {
		mDelta["usage"] = map[string]interface{}{"output_tokens": finalUsage.CompletionTokens}
	}
	SendEvent(w, "message_delta", mDelta)
	SendEvent(w, "message_stop", map[string]interface{}{"type": "message_stop"})
	flusher.Flush()
}

func findFinishReason(delta types.OpenAICSDelta) string {
	if len(delta.Choices) > 0 {
		return delta.Choices[0].FinishReason
	}
	return ""
}

func SendEvent(w io.Writer, event string, data interface{}) {
	b, _ := json.Marshal(data)
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, string(b))
}
