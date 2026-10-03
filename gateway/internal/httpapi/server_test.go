package httpapi

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/arnabry11/ev-charging/gateway/internal/registry"
	"github.com/arnabry11/ev-charging/gateway/internal/session"
	"github.com/arnabry11/ev-charging/gateway/internal/signing"
)

func TestHealth(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	NewRouter(Dependencies{Chargers: registry.New()}).ServeHTTP(rec, req)

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
	NewRouter(Dependencies{Chargers: reg}).ServeHTTP(rec, req)
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
	NewRouter(Dependencies{Chargers: reg}).ServeHTTP(missRec, missing)
	if missRec.Code != http.StatusNotFound {
		t.Fatalf("missing status = %d", missRec.Code)
	}
}

func TestStartSessionRequiresValidSignature(t *testing.T) {
	t.Parallel()

	body := []byte(`{
		"command_id":"6f1c7a52-9c21-4c0e-8e1c-0a9f7d1c2b11",
		"session_ref":"b3a0f1d2-1111-4444-8888-123456789abc",
		"charger_id":"CHG-MUM-0001",
		"connector_id":1,
		"id_tag":"S-b3a0f1d2",
		"limits":{"max_energy_wh":12000,"max_duration_s":5400},
		"expires_at":"2099-10-03T08:05:00Z"
	}`)
	service := &fakeCommandService{
		startResponse: session.Response{
			Status: http.StatusAccepted,
			Result: session.Result{State: session.StateStartRequested},
		},
	}
	router := NewRouter(Dependencies{
		Chargers:      registry.New(),
		Sessions:      service,
		SigningSecret: "secret",
	})

	unsigned := httptest.NewRequest(http.MethodPost, "/internal/v1/commands/start-session", bytes.NewReader(body))
	unsignedResponse := httptest.NewRecorder()
	router.ServeHTTP(unsignedResponse, unsigned)
	if unsignedResponse.Code != http.StatusUnauthorized {
		t.Fatalf("unsigned status = %d", unsignedResponse.Code)
	}

	timestamp := time.Now().UTC().Format(time.RFC3339)
	signed := httptest.NewRequest(http.MethodPost, "/internal/v1/commands/start-session", bytes.NewReader(body))
	signed.Header.Set(signing.TimestampHeader, timestamp)
	signed.Header.Set(signing.SignatureHeader, testSignature("secret", timestamp, body))
	signedResponse := httptest.NewRecorder()
	router.ServeHTTP(signedResponse, signed)
	if signedResponse.Code != http.StatusAccepted {
		t.Fatalf("signed status = %d body=%s", signedResponse.Code, signedResponse.Body.String())
	}
	if service.startCalls != 1 {
		t.Fatalf("start calls = %d", service.startCalls)
	}
}

type fakeCommandService struct {
	startResponse session.Response
	startCalls    int
}

func (f *fakeCommandService) Start(context.Context, session.StartRequest) (session.Response, error) {
	f.startCalls++
	return f.startResponse, nil
}

func (f *fakeCommandService) Stop(context.Context, session.StopRequest) (session.Response, error) {
	return session.Response{}, nil
}

func testSignature(secret, timestamp string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(timestamp))
	_, _ = mac.Write([]byte("."))
	_, _ = mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}
