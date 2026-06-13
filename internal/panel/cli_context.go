package panel

import (
	"errors"
	"net/http"

	"github.com/bloodstalk1/arkroute/internal/clisetup"
)

func handleCLIContext(path string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if rejectIfNotMethod(w, r, http.MethodGet) {
			return
		}
		cfg, err := loadOrBootstrapConfig(path)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"schema_version": 1, "error": err.Error()})
			return
		}
		ctx, err := clisetup.BuildContext(cfg, clisetup.Request{
			ModelID:    r.URL.Query().Get("model_id"),
			RouteAlias: r.URL.Query().Get("route_alias"),
		})
		if err != nil {
			writeJSON(w, cliContextStatus(err), map[string]any{"schema_version": 1, "error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, ctx)
	}
}

func cliContextStatus(err error) int {
	if errors.Is(err, clisetup.ErrModelNotFound) || errors.Is(err, clisetup.ErrRouteNotFound) {
		return http.StatusNotFound
	}
	return http.StatusBadRequest
}
