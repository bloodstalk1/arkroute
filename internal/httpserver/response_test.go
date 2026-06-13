package httpserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWriteJSONSetsContentTypeAndStatus(t *testing.T) {
	rr := httptest.NewRecorder()
	WriteJSON(rr, http.StatusCreated, map[string]string{"hello": "world"})

	if got, want := rr.Code, http.StatusCreated; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
	if got, want := rr.Header().Get("Content-Type"), "application/json"; got != want {
		t.Fatalf("Content-Type = %q, want %q", got, want)
	}
	var decoded map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&decoded); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if decoded["hello"] != "world" {
		t.Fatalf("body = %v", decoded)
	}
}

func TestWriteErrorWritesEnvelope(t *testing.T) {
	rr := httptest.NewRecorder()
	WriteError(rr, http.StatusBadRequest, "bad input")

	if got, want := rr.Code, http.StatusBadRequest; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
	var decoded map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&decoded); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if decoded["error"] != "bad input" {
		t.Fatalf("body = %v", decoded)
	}
}

func TestWriteJSONEncodesNestedValues(t *testing.T) {
	rr := httptest.NewRecorder()
	type payload struct {
		Items []int `json:"items"`
	}
	WriteJSON(rr, http.StatusOK, payload{Items: []int{1, 2, 3}})

	var got payload
	if err := json.NewDecoder(rr.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Items) != 3 || got.Items[0] != 1 {
		t.Fatalf("items = %v", got.Items)
	}
}
