package anthropic

import "encoding/json"

func DecodeMessageRequest(body []byte) (MessageRequest, error) {
	var req MessageRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return MessageRequest{}, err
	}
	return req, nil
}

func EncodeError(errorType string, message string) ([]byte, error) {
	return json.Marshal(map[string]any{
		"type": "error",
		"error": map[string]string{
			"type":    errorType,
			"message": message,
		},
	})
}
