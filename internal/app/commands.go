package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"bat.dev/arkrouter/internal/config"
)

func ValidateConfig(path string, w io.Writer) error {
	cfg, err := config.LoadFile(pathOrDefault(path))
	if err != nil {
		return err
	}
	if err := cfg.Validate(); err != nil {
		return err
	}
	fmt.Fprintln(w, "config ok")
	return nil
}

func Doctor(path string, w io.Writer) error {
	cfg, err := config.LoadFile(pathOrDefault(path))
	if err != nil {
		return err
	}
	if err := cfg.Validate(); err != nil {
		return err
	}
	fmt.Fprintf(w, "config: ok\nproviders: %d\nmodels: %d\nroutes: %d\n", len(cfg.Providers), len(cfg.Models), len(cfg.Routes))
	return nil
}

func PrintLogs(path string, w io.Writer) error {
	if path == "" {
		path = DefaultLogPath()
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}

func TestRoute(path string, model string, prompt string, w io.Writer) error {
	cfg, err := config.LoadFile(pathOrDefault(path))
	if err != nil {
		return err
	}
	body := map[string]any{
		"model":      model,
		"max_tokens": 128,
		"messages": []map[string]any{{
			"role": "user",
			"content": []map[string]string{{
				"type": "text",
				"text": prompt,
			}},
		}},
	}
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	url := fmt.Sprintf("http://%s:%d/v1/messages", cfg.Server.Host, cfg.Server.Port)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+cfg.Server.ClientKey)
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("gateway returned %d: %s", resp.StatusCode, string(respBody))
	}
	_, err = w.Write(respBody)
	if err == nil {
		_, err = fmt.Fprintln(w)
	}
	return err
}

func pathOrDefault(path string) string {
	if path != "" {
		return path
	}
	return DefaultConfigPath()
}
