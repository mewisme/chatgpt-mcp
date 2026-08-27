package admin

import (
	"encoding/json"
	"net/http"
)

type API struct{}

func NewAPI() *API { return &API{} }

func (a *API) Health(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
}
