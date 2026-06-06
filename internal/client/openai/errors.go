package openai

import (
	"net/http"

	arkruntime "github.com/bloodstalk1/arkroute/internal/runtime"
)

type errorBody struct {
	Error errorDetail `json:"error"`
}

type errorDetail struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Param   string `json:"param,omitempty"`
	Code    string `json:"code,omitempty"`
}

func writeOpenAIError(w http.ResponseWriter, status int, errorType string, code string, param string, message string) {
	writeJSON(w, status, errorBody{Error: errorDetail{Message: message, Type: errorType, Param: param, Code: code}})
}

func writeExecutionError(w http.ResponseWriter, err error) {
	var execErr *arkruntime.ExecutionError
	if arkruntime.AsExecutionError(err, &execErr) {
		status := http.StatusBadGateway
		errorType := "api_error"
		code := string(execErr.Class)
		param := ""
		switch execErr.Class {
		case arkruntime.ErrorRouteNotFound:
			status = http.StatusNotFound
			errorType = "invalid_request_error"
			code = "route_not_found"
			param = "model"
		case arkruntime.ErrorInvalidRequest:
			status = http.StatusBadRequest
			errorType = "invalid_request_error"
		case arkruntime.ErrorUnsupportedCapability:
			status = http.StatusBadRequest
			errorType = "invalid_request_error"
			code = "unsupported_capability"
		case arkruntime.ErrorUpstreamRateLimit:
			status = http.StatusTooManyRequests
			errorType = "rate_limit_error"
		case arkruntime.ErrorUpstreamAuth:
			status = http.StatusForbidden
			errorType = "authentication_error"
		}
		writeOpenAIError(w, status, errorType, code, param, execErr.Message)
		return
	}
	writeOpenAIError(w, http.StatusBadGateway, "api_error", "upstream_error", "", err.Error())
}
