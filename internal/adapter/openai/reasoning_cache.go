package openai

import (
	"sync"
	"time"

	oaiproto "github.com/bloodstalk1/arkroute/internal/protocol/openai"
)

type reasoningCacheEntry struct {
	reasoning string
	storedAt  time.Time
	sequence  uint64
}

var (
	reasoningCacheMu         sync.Mutex
	reasoningByToolCall      = map[string]reasoningCacheEntry{}
	reasoningCacheSequence   uint64
	reasoningCacheTTL        = 2 * time.Hour
	reasoningCacheMaxEntries = 2048
)

func rememberReasoningForToolCalls(reasoning string, calls []oaiproto.ToolCall) {
	if reasoning == "" {
		return
	}
	for _, call := range calls {
		rememberReasoningForToolCall(call, reasoning)
	}
}

func rememberReasoningForToolCall(call oaiproto.ToolCall, reasoning string) {
	key := reasoningCacheKeyForToolCall(call)
	if key == "" || reasoning == "" {
		return
	}
	now := time.Now()
	reasoningCacheMu.Lock()
	defer reasoningCacheMu.Unlock()
	pruneExpiredReasoningLocked(now)
	reasoningCacheSequence++
	reasoningByToolCall[key] = reasoningCacheEntry{reasoning: reasoning, storedAt: now, sequence: reasoningCacheSequence}
	evictReasoningOverflowLocked()
}

func lookupReasoningForToolCalls(calls []oaiproto.ToolCall) string {
	now := time.Now()
	reasoningCacheMu.Lock()
	defer reasoningCacheMu.Unlock()
	for _, call := range calls {
		key := reasoningCacheKeyForToolCall(call)
		if key == "" {
			continue
		}
		entry, ok := reasoningByToolCall[key]
		if !ok {
			continue
		}
		if isReasoningExpired(entry, now) {
			delete(reasoningByToolCall, key)
			continue
		}
		return entry.reasoning
	}
	return ""
}

func reasoningCacheKeyForToolCall(call oaiproto.ToolCall) string {
	if call.ID == "" {
		return ""
	}
	return call.ID + "\x00" + call.Function.Name + "\x00" + call.Function.Arguments
}

func pruneExpiredReasoningLocked(now time.Time) {
	for id, entry := range reasoningByToolCall {
		if isReasoningExpired(entry, now) {
			delete(reasoningByToolCall, id)
		}
	}
}

func evictReasoningOverflowLocked() {
	for reasoningCacheMaxEntries >= 0 && len(reasoningByToolCall) > reasoningCacheMaxEntries {
		var oldestID string
		var oldestSequence uint64
		for id, entry := range reasoningByToolCall {
			if oldestID == "" || entry.sequence < oldestSequence {
				oldestID = id
				oldestSequence = entry.sequence
			}
		}
		if oldestID == "" {
			return
		}
		delete(reasoningByToolCall, oldestID)
	}
}

func isReasoningExpired(entry reasoningCacheEntry, now time.Time) bool {
	return reasoningCacheTTL >= 0 && now.Sub(entry.storedAt) > reasoningCacheTTL
}

func resetReasoningCacheForTest() {
	reasoningCacheMu.Lock()
	defer reasoningCacheMu.Unlock()
	reasoningByToolCall = map[string]reasoningCacheEntry{}
	reasoningCacheSequence = 0
}
