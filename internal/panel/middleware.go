package panel

import "net/http"

// rejectIfNotMethod writes a 405 JSON response and returns true when the
// request's method does not match allowed. Handlers should early-return
// when this is true:
//
//	if rejectIfNotMethod(w, r, http.MethodPost) {
//	    return
//	}
func rejectIfNotMethod(w http.ResponseWriter, r *http.Request, allowed string) bool {
	if r.Method == allowed {
		return false
	}
	w.Header().Set("Allow", allowed)
	writeJSON(w, http.StatusMethodNotAllowed, map[string]any{
		"schema_version": 1,
		"error":          "method not allowed",
	})
	return true
}

// writeMethodNotAllowed is the variant for switch-style dispatch where the
// caller already knows the Allow header value (which may be a comma-joined
// list of methods).
func writeMethodNotAllowed(w http.ResponseWriter, allow string) {
	w.Header().Set("Allow", allow)
	writeJSON(w, http.StatusMethodNotAllowed, map[string]any{
		"schema_version": 1,
		"error":          "method not allowed",
	})
}
