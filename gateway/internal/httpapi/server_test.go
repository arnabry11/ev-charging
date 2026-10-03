package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/arnabry11/ev-charging/gateway/internal/registry"
)

func TestHealth(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	NewRouter(registry.New()).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body["status"] != "ok" {
		t.Fatalf("status = %q, want %q", body["status"], "ok")
	}
}

func TestShowCharger(t *testing.T) {
	t.Parallel()

	reg := registry.New()
	reg.Booted("CHG-MUM-0001", "POCSim", "ev-charging", "0.1", 10)
	reg.ConnectorStatus("CHG-MUM-0001", registry.Connector{
		ID:        1,
		Status:    "Available",
		ErrorCode: "NoError",
		VendorID:  "POCSim",
	}, time.Now())

	req := httptest.NewRequest(http.MethodGet, "/internal/v1/chargers/CHG-MUM-0001", nil)
	rec := httptest.NewRecorder()
	NewRouter(reg).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["connection_state"] != registry.StateConnected {
		t.Fatalf("connection_state = %v", body["connection_state"])
	}
	if body["vendor"] != "POCSim" {
		t.Fatalf("vendor = %v", body["vendor"])
	}
	connectors := body["connectors"].([]any)
	connector := connectors[0].(map[string]any)
	if connector["status"] != "Available" {
		t.Fatalf("connector = %v", connector)
	}
	if connector["vendor_id"] != "POCSim" {
		t.Fatalf("connector = %v", connector)
	}
	if _, err := time.Parse(time.RFC3339, body["last_boot_at"].(string)); err != nil {
		t.Fatalf("last_boot_at: %v", err)
	}

	missing := httptest.NewRequest(http.MethodGet, "/internal/v1/chargers/missing", nil)
	missRec := httptest.NewRecorder()
	NewRouter(reg).ServeHTTP(missRec, missing)
	if missRec.Code != http.StatusNotFound {
		t.Fatalf("missing status = %d", missRec.Code)
	}
}
