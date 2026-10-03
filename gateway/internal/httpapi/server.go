package httpapi

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/arnabry11/ev-charging/gateway/internal/registry"
	"github.com/go-chi/chi/v5"
)

type chargerLookup interface {
	Get(id string) (registry.Charger, bool)
}

func NewRouter(chargers chargerLookup) http.Handler {
	r := chi.NewRouter()
	r.Get("/health", health)
	r.Get("/internal/v1/chargers/{chargerID}", showCharger(chargers))
	return r
}

func health(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func showCharger(chargers chargerLookup) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "chargerID")
		charger, ok := chargers.Get(id)
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(chargerView(charger))
	}
}

type chargerJSON struct {
	ChargerID           string `json:"charger_id"`
	ConnectionState     string `json:"connection_state"`
	Vendor              string `json:"vendor,omitempty"`
	Model               string `json:"model,omitempty"`
	FirmwareVersion     string `json:"firmware_version,omitempty"`
	LastSeenAt          string `json:"last_seen_at"`
	LastBootAt          string `json:"last_boot_at,omitempty"`
	HeartbeatIntervalS  int    `json:"heartbeat_interval_s,omitempty"`
}

func chargerView(c registry.Charger) chargerJSON {
	view := chargerJSON{
		ChargerID:          c.ID,
		ConnectionState:    c.ConnectionState,
		Vendor:             c.Vendor,
		Model:              c.Model,
		FirmwareVersion:    c.FirmwareVersion,
		HeartbeatIntervalS: c.HeartbeatInterval,
	}
	if !c.LastSeenAt.IsZero() {
		view.LastSeenAt = c.LastSeenAt.UTC().Format(time.RFC3339)
	}
	if !c.LastBootAt.IsZero() {
		view.LastBootAt = c.LastBootAt.UTC().Format(time.RFC3339)
	}
	return view
}
