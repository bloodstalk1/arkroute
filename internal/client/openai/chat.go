package openai

import (
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/bloodstalk1/arkroute/internal/protocol"
	oaiproto "github.com/bloodstalk1/arkroute/internal/protocol/openai"
	arkruntime "github.com/bloodstalk1/arkroute/internal/runtime"
)

type chatCompletionResponse struct {
	ID      string                 `json:"id"`
	Object  string                 `json:"object"`
	Created int64                  `json:"created"`
	Model   string                 `json:"model"`
	Choices []chatCompletionChoice `json:"choices"`
	Usage   chatCompletionUsage    `json:"usage"`
}

type chatCompletionChoice struct {
	Index        int                   `json:"index"`
	Message      chatCompletionMessage `json:"message"`
	FinishReason string                `json:"finish_reason"`
}

type chatCompletionMessage struct {
	Role      string              `json:"role"`
	Content   string              `json:"content,omitempty"`
	ToolCalls []oaiproto.ToolCall `json:"tool_calls,omitempty"`
}

type chatCompletionUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

func (s *Server) handleChatCompletions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeOpenAIError(w, http.StatusMethodNotAllowed, "invalid_request_error", "method_not_allowed", "", "method not allowed")
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 50<<20))
	if err != nil {
		writeOpenAIError(w, http.StatusBadRequest, "invalid_request_error", "read_failed", "", "read request failed")
		return
	}
	chatReq, err := oaiproto.DecodeChatRequest(body)
	if err != nil {
		writeOpenAIError(w, http.StatusBadRequest, "invalid_request_error", "invalid_json", "", "invalid OpenAI chat request")
		return
	}
	normalized, requirements, err := oaiproto.NormalizeChatRequest(chatReq)
	if err != nil {
		writeOpenAIError(w, http.StatusBadRequest, "invalid_request_error", "unsupported_feature", "", err.Error())
		return
	}
	gen := generationFromRequest(r)
	if gen == nil {
		writeOpenAIError(w, http.StatusInternalServerError, "api_error", "missing_generation", "", "missing runtime generation")
		return
	}
	if chatReq.Stream {
		stream, err := gen.Stream(r.Context(), arkruntime.ExecuteRequest{
			RequestID:    requestID(r),
			Client:       "openai-chat",
			Model:        chatReq.Model,
			Requirements: requirements,
			Request:      normalized,
		})
		if err != nil {
			writeExecutionError(w, err)
			return
		}
		defer func() { _ = stream.Close() }()
		writeChatCompletionStream(w, stream, chatReq.Model)
		return
	}
	result, err := gen.Execute(r.Context(), arkruntime.ExecuteRequest{
		RequestID:    requestID(r),
		Client:       "openai-chat",
		Model:        chatReq.Model,
		Requirements: requirements,
		Request:      normalized,
	})
	if err != nil {
		writeExecutionError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, mapChatCompletionResponse(result.Response, chatReq.Model))
}

func requestID(r *http.Request) string {
	if value := r.Header.Get("x-request-id"); value != "" {
		return value
	}
	return newOpenAIID("req_")
}

func mapChatCompletionResponse(resp protocol.Response, model string) chatCompletionResponse {
	message := chatCompletionMessage{Role: "assistant"}
	var textParts []string
	for _, block := range resp.Content {
		switch block.Type {
		case "text":
			if block.Text != "" {
				textParts = append(textParts, block.Text)
			}
		case "tool_use":
			message.ToolCalls = append(message.ToolCalls, oaiproto.ToolCall{
				ID:   block.ID,
				Type: "function",
				Function: oaiproto.FunctionCall{
					Name:      block.Name,
					Arguments: string(block.Input),
				},
			})
		}
	}
	message.Content = strings.Join(textParts, "\n")
	return chatCompletionResponse{
		ID:      responseID(resp.ID),
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   model,
		Choices: []chatCompletionChoice{{
			Index:        0,
			Message:      message,
			FinishReason: mapStopReason(resp.StopReason),
		}},
		Usage: chatCompletionUsage{
			PromptTokens:     resp.Usage.InputTokens,
			CompletionTokens: resp.Usage.OutputTokens,
			TotalTokens:      resp.Usage.InputTokens + resp.Usage.OutputTokens,
		},
	}
}

func responseID(id string) string {
	if id != "" {
		return id
	}
	return newOpenAIID("chatcmpl_")
}

func mapStopReason(reason string) string {
	switch reason {
	case "tool_use":
		return "tool_calls"
	case "max_tokens":
		return "length"
	default:
		return "stop"
	}
}
