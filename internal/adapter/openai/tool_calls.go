package openai

import (
	"encoding/json"

	oaiproto "github.com/bloodstalk1/arkroute/internal/protocol/openai"
)

func normalizeToolCall(call oaiproto.ToolCall) oaiproto.ToolCall {
	name, arguments := normalizeToolNameAndArguments(call.Function.Name, call.Function.Arguments)
	call.Function.Name = name
	call.Function.Arguments = arguments
	return call
}

func normalizeToolNameAndArguments(name string, arguments string) (string, string) {
	var decoded struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if json.Unmarshal([]byte(name), &decoded) != nil || decoded.Name == "" {
		return name, arguments
	}
	normalizedArgs := arguments
	if len(decoded.Arguments) > 0 && string(decoded.Arguments) != "null" {
		var argString string
		if json.Unmarshal(decoded.Arguments, &argString) == nil {
			normalizedArgs = argString
		} else {
			normalizedArgs = string(decoded.Arguments)
		}
	}
	return decoded.Name, normalizedArgs
}
