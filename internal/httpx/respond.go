package httpx

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

// JSON writes v as an application/json response with the given status code.
func JSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if v == nil {
		return
	}
	if err := json.NewEncoder(w).Encode(v); err != nil {
		// Status/headers are already flushed; we can only log.
		slog.Error("httpx: encode response", "error", err)
	}
}

// errorBody is the single error envelope every handler returns.
type errorBody struct {
	Error string `json:"error"`
}

// Error writes a JSON error envelope with the given status code and message.
func Error(w http.ResponseWriter, status int, msg string) {
	JSON(w, status, errorBody{Error: msg})
}
