package panel

import (
	"net/http"

	"github.com/bloodstalk1/arkroute/internal/clitools"
)

type CLIToolsService interface {
	Status() (clitools.ListResponse, error)
	LaunchClaude() (clitools.LaunchResponse, error)
}

func handleCLIToolsStatus(service CLIToolsService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"schema_version": 1, "error": "method not allowed"})
			return
		}
		if service == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{"schema_version": 1, "error": "cli tools unavailable"})
			return
		}
		resp, err := service.Status()
		if err != nil {
			writeCLIToolsError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, resp)
	}
}

func handleClaudeLaunch(service CLIToolsService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"schema_version": 1, "error": "method not allowed"})
			return
		}
		if service == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{"schema_version": 1, "error": "cli tools unavailable"})
			return
		}
		resp, err := service.LaunchClaude()
		if err != nil {
			writeCLIToolsError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, resp)
	}
}

func writeCLIToolsError(w http.ResponseWriter, err error) {
	if cliErr, ok := err.(*clitools.Error); ok {
		writeJSON(w, cliErr.Status, map[string]any{
			"schema_version": 1,
			"code":           cliErr.Code,
			"error":          cliErr.Message,
			"remediation":    cliErr.Remediation,
		})
		return
	}
	writeJSON(w, http.StatusInternalServerError, map[string]any{"schema_version": 1, "error": err.Error()})
}
