package web

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
)

// runTokenHeader carries the agent-launch token on run/cancel requests.
const runTokenHeader = "X-Run-Token"

// checkRunToken gates agent-control endpoints when a run token is configured.
// Unlike the CSRF token it is never served to browsers: it has to come from
// the operator (the serve console) or the human at the UI, so merely being
// able to reach the port is not enough to launch an agent. Returns false
// after writing a 401 the UI recognizes ({"need_run_token": true}) to
// prompt for the token once.
func checkRunToken(w http.ResponseWriter, r *http.Request, token string) bool {
	if token == "" {
		return true
	}
	got := r.Header.Get(runTokenHeader)
	if subtle.ConstantTimeCompare([]byte(got), []byte(token)) == 1 {
		return true
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	_ = json.NewEncoder(w).Encode(map[string]any{"need_run_token": true})
	return false
}
