package openai

import (
	"net/url"
	"strings"
)

func ChatCompletionsURL(base string) (string, error) {
	parsed, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	path := strings.TrimRight(parsed.Path, "/")
	if path == "" {
		path = "/v1"
	}
	if strings.HasSuffix(path, "/chat/completions") {
		parsed.Path = path
		return parsed.String(), nil
	}
	if isOpenCodeGoBase(parsed.Host, path) {
		parsed.Path = path + "/v1/chat/completions"
		return parsed.String(), nil
	}
	parsed.Path = path + "/chat/completions"
	return parsed.String(), nil
}

func isOpenCodeGoBase(host string, path string) bool {
	host = strings.ToLower(host)
	path = strings.ToLower(path)
	return strings.Contains(host, "opencode.ai") && strings.HasSuffix(path, "/zen/go")
}
