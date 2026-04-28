package translator

import (
	"encoding/json"
	"proxy-ai/internal/types"
	"strings"
)

func ContentToString(content interface{}) string {
	switch v := content.(type) {
	case string:
		return v
	case []interface{}:
		var parts []string
		for _, part := range v {
			if m, ok := part.(map[string]interface{}); ok {
				if t, ok := m["type"].(string); ok && t == "text" {
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

func Stringify(v interface{}) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func TranslateMessages(antMsgs []types.AnthropicMessage) []types.OpenAIMessage {
	var oaMsgs []types.OpenAIMessage
	for _, m := range antMsgs {
		oaMsg := types.OpenAIMessage{Role: m.Role}
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
						oaMsg.ToolCalls = append(oaMsg.ToolCalls, types.OpenAIToolCall{
							ID:   mPart["id"].(string),
							Type: "function",
							Function: struct {
								Name      string `json:"name"`
								Arguments string `json:"arguments"`
							}{
								Name:      mPart["name"].(string),
								Arguments: Stringify(mPart["input"]),
							},
						})
					case "tool_result":
						oaMsg.Role = "tool"
						oaMsg.ToolCallID = mPart["tool_use_id"].(string)
						oaMsg.Content = mPart["content"]
					}
				}
			}
			if len(textParts) > 0 {
				oaMsg.Content = strings.Join(textParts, "")
			}
		}
		oaMsgs = append(oaMsgs, oaMsg)
	}
	return oaMsgs
}
