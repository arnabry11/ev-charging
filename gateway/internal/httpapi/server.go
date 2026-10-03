package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"slices"
	"time"

	"github.com/arnabry11/ev-charging/gateway/internal/registry"
	"github.com/arnabry11/ev-charging/gateway/internal/session"
	"github.com/arnabry11/ev-charging/gateway/internal/signing"
	"github.com/go-chi/chi/v5"
)

type chargerLookup interface {
	Get(id string) (registry.Charger, bool)
}

type commandService interface {
	Start(context.Context, session.StartRequest) (session.Response, error)
	Stop(context.Context, session.StopRequest) (session.Response, error)
}

type Dependencies struct {
	Chargers      chargerLookup
	Sessions      commandService
	SigningSecret string
}

func NewRouter(deps Dependencies) http.Handler {
	r := chi.NewRouter()
	r.Get("/health", health)
	r.Get("/internal/v1/chargers/{chargerID}", showCharger(deps.Chargers))
	if deps.Sessions != nil {
		r.Route("/internal/v1/commands", func(r chi.Router) {
			r.Use(signing.New(deps.SigningSecret, 5*time.Minute).Verify)
			r.Post("/start-session", startSession(deps.Sessions))
			r.Post("/stop-session", stopSession(deps.Sessions))
		})
	}
	return r
}

func health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func startSession(sessions commandService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var request session.StartRequest
		if err := decodeJSON(r, &request); err != nil {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "invalid_request", "detail": err.Error()})
			return
		}
		response, err := sessions.Start(r.Context(), request)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal_error"})
			return
		}
		writeJSON(w, response.Status, response.Result)
	}
}

func stopSession(sessions commandService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var request session.StopRequest
		if err := decodeJSON(r, &request); err != nil {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "invalid_request", "detail": err.Error()})
			return
		}
		response, err := sessions.Stop(r.Context(), request)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal_error"})
			return
		}
		writeJSON(w, response.Status, response.Result)
	}
}

func decodeJSON(r *http.Request, target any) error {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("request body must contain one JSON object")
		}
		return err
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func showCharger(chargers chargerLookup) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "chargerID")
		charger, ok := chargers.Get(id)
		if !ok {
			http.NotFound(w, r)
			return
		}
		writeJSON(w, http.StatusOK, chargerView(charger))
	}
}

type chargerJSON struct {
	ChargerID          string          `json:"charger_id"`
	ConnectionState    string          `json:"connection_state"`
	Vendor             string          `json:"vendor,omitempty"`
	Model              string          `json:"model,omitempty"`
	FirmwareVersion    string          `json:"firmware_version,omitempty"`
	LastSeenAt         string          `json:"last_seen_at"`
	LastBootAt         string          `json:"last_boot_at,omitempty"`
	HeartbeatIntervalS int             `json:"heartbeat_interval_s,omitempty"`
	Connectors         []connectorJSON `json:"connectors"`
}

type connectorJSON struct {
	ConnectorID     int    `json:"connector_id"`
	Status          string `json:"status"`
	ErrorCode       string `json:"error_code"`
	Info            string `json:"info,omitempty"`
	VendorID        string `json:"vendor_id,omitempty"`
	VendorErrorCode string `json:"vendor_error_code,omitempty"`
	UpdatedAt       string `json:"updated_at"`
}

func chargerView(c registry.Charger) chargerJSON {
	view := chargerJSON{
		ChargerID:          c.ID,
		ConnectionState:    c.ConnectionState,
		Vendor:             c.Vendor,
		Model:              c.Model,
		FirmwareVersion:    c.FirmwareVersion,
		HeartbeatIntervalS: c.HeartbeatInterval,
		Connectors:         make([]connectorJSON, 0, len(c.Connectors)),
	}
	connectorIDs := make([]int, 0, len(c.Connectors))
	for id := range c.Connectors {
		connectorIDs = append(connectorIDs, id)
	}
	slices.Sort(connectorIDs)
	for _, id := range connectorIDs {
		connector := c.Connectors[id]
		view.Connectors = append(view.Connectors, connectorJSON{
			ConnectorID:     connector.ID,
			Status:          connector.Status,
			ErrorCode:       connector.ErrorCode,
			Info:            connector.Info,
			VendorID:        connector.VendorID,
			VendorErrorCode: connector.VendorErrorCode,
			UpdatedAt:       connector.UpdatedAt.UTC().Format(time.RFC3339),
		})
	}
	if !c.LastSeenAt.IsZero() {
		view.LastSeenAt = c.LastSeenAt.UTC().Format(time.RFC3339)
	}
	if !c.LastBootAt.IsZero() {
		view.LastBootAt = c.LastBootAt.UTC().Format(time.RFC3339)
	}
	return view
}
