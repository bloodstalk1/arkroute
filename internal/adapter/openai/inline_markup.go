package openai

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bloodstalk1/arkroute/internal/protocol"
)

func splitInlineThinking(content string) (string, string, bool) {
	trimmed := strings.TrimLeft(content, " \t\r\n")
	lower := strings.ToLower(trimmed)
	if !strings.HasPrefix(lower, "<think>") {
		return "", content, false
	}
	closeTag := "</think>"
	end := strings.Index(lower, closeTag)
	if end < 0 {
		return "", content, false
	}
	thinking := strings.TrimSpace(trimmed[len("<think>"):end])
	rest := strings.TrimLeft(trimmed[end+len(closeTag):], " \t\r\n")
	return thinking, rest, true
}

func parsePseudoToolUse(content string) (protocol.ContentBlock, bool) {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return protocol.ContentBlock{}, false
	}
	if body, ok := extractWholeTag(trimmed, "write"); ok {
		path, hasPath := extractFirstTag(body, "path")
		if !hasPath {
			path, hasPath = extractFirstTag(body, "file_path")
		}
		content, hasContent := extractFirstTag(body, "content")
		if !hasPath || !hasContent {
			return protocol.ContentBlock{}, false
		}
		return syntheticToolBlock("Write", map[string]string{
			"file_path": strings.TrimSpace(path),
			"content":   content,
		}), true
	}
	if body, ok := extractWholeTag(trimmed, "tool_call"); ok {
		if name, invokeBody, ok := extractInvoke(body); ok {
			return syntheticToolBlock(name, parseSimpleTagMap(invokeBody)), true
		}
	}
	if name, body, ok := extractInvoke(trimmed); ok {
		return syntheticToolBlock(name, parseSimpleTagMap(body)), true
	}
	return protocol.ContentBlock{}, false
}

func syntheticToolBlock(name string, input map[string]string) protocol.ContentBlock {
	input = normalizePseudoToolInput(name, input)
	data, _ := json.Marshal(input)
	return protocol.ContentBlock{
		Type:  "tool_use",
		ID:    syntheticToolID(name, data),
		Name:  name,
		Input: data,
	}
}

func normalizePseudoToolInput(name string, input map[string]string) map[string]string {
	out := make(map[string]string, len(input))
	for key, value := range input {
		normalized := strings.TrimSpace(key)
		switch strings.ToLower(name) {
		case "write", "edit", "multiedit", "read":
			if normalized == "path" {
				normalized = "file_path"
			}
		case "bash":
			if normalized == "cmd" || normalized == "shell" {
				normalized = "command"
			}
		}
		if strings.EqualFold(normalized, "old") {
			normalized = "old_string"
		}
		if strings.EqualFold(normalized, "new") {
			normalized = "new_string"
		}
		out[normalized] = value
	}
	return out
}

func syntheticToolID(name string, input []byte) string {
	sum := sha256.Sum256(append([]byte(name+"\x00"), input...))
	return "toolu_arkroute_" + hex.EncodeToString(sum[:])[:16]
}

func extractWholeTag(content string, tag string) (string, bool) {
	open := "<" + tag + ">"
	close := "</" + tag + ">"
	lower := strings.ToLower(content)
	if !strings.HasPrefix(lower, open) {
		return "", false
	}
	end := strings.LastIndex(lower, close)
	if end < len(open) {
		return "", false
	}
	after := strings.TrimSpace(content[end+len(close):])
	if after != "" {
		return "", false
	}
	return content[len(open):end], true
}

func extractFirstTag(content string, tag string) (string, bool) {
	open := "<" + strings.ToLower(tag) + ">"
	close := "</" + strings.ToLower(tag) + ">"
	lower := strings.ToLower(content)
	start := strings.Index(lower, open)
	if start < 0 {
		return "", false
	}
	valueStart := start + len(open)
	endRel := strings.Index(lower[valueStart:], close)
	if endRel < 0 {
		return "", false
	}
	return content[valueStart : valueStart+endRel], true
}

func extractInvoke(content string) (string, string, bool) {
	lower := strings.ToLower(content)
	start := strings.Index(lower, "<invoke")
	if start < 0 {
		return "", "", false
	}
	openEndRel := strings.Index(content[start:], ">")
	if openEndRel < 0 {
		return "", "", false
	}
	openEnd := start + openEndRel
	openTag := content[start+len("<invoke") : openEnd]
	name, ok := extractNameAttribute(openTag)
	if !ok || name == "" {
		return "", "", false
	}
	closeTag := "</invoke>"
	bodyStart := openEnd + 1
	closeRel := strings.Index(strings.ToLower(content[bodyStart:]), closeTag)
	if closeRel < 0 {
		return "", "", false
	}
	after := strings.TrimSpace(content[bodyStart+closeRel+len(closeTag):])
	if after != "" {
		return "", "", false
	}
	return name, content[bodyStart : bodyStart+closeRel], true
}

func extractNameAttribute(attrs string) (string, bool) {
	attrs = strings.TrimSpace(attrs)
	for _, quote := range []byte{'"', '\''} {
		prefix := fmt.Sprintf("name=%c", quote)
		index := strings.Index(attrs, prefix)
		if index < 0 {
			continue
		}
		valueStart := index + len(prefix)
		valueEndRel := strings.IndexByte(attrs[valueStart:], quote)
		if valueEndRel < 0 {
			return "", false
		}
		return attrs[valueStart : valueStart+valueEndRel], true
	}
	return "", false
}

func parseSimpleTagMap(content string) map[string]string {
	out := map[string]string{}
	remaining := content
	for {
		openStart := strings.Index(remaining, "<")
		if openStart < 0 {
			break
		}
		openEndRel := strings.Index(remaining[openStart:], ">")
		if openEndRel < 0 {
			break
		}
		openEnd := openStart + openEndRel
		tag := strings.TrimSpace(remaining[openStart+1 : openEnd])
		if tag == "" || strings.HasPrefix(tag, "/") || strings.Contains(tag, " ") {
			remaining = remaining[openEnd+1:]
			continue
		}
		closeTag := "</" + strings.ToLower(tag) + ">"
		valueStart := openEnd + 1
		closeRel := strings.Index(strings.ToLower(remaining[valueStart:]), closeTag)
		if closeRel < 0 {
			remaining = remaining[openEnd+1:]
			continue
		}
		out[tag] = remaining[valueStart : valueStart+closeRel]
		remaining = remaining[valueStart+closeRel+len(closeTag):]
	}
	return out
}
