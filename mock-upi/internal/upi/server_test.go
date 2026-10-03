package upi

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCreateIsIdempotentAndWebhookCanBeReplayed(t *testing.T) {
	t.Parallel()

	var deliveries [][]byte
	webhook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if r.Header.Get("X-Signature") != Sign([]byte("secret"), r.Header.Get("X-Timestamp"), body) {
			t.Errorf("bad signature")
		}
		deliveries = append(deliveries, body)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(webhook.Close)

	server := NewServer(NewStore("secret"), webhook.URL, "secret", nil)
	body := []byte(`{"payment_id":"6f1c7a52-9c21-4c0e-8e1c-0a9f7d1c2b11","amount_paise":10000,"idempotency_key":"pay-1"}`)
	first := post(server, "/payments", body)
	if first.Code != http.StatusCreated {
		t.Fatalf("create status = %d body=%s", first.Code, first.Body.String())
	}
	replay := post(server, "/payments", body)
	if replay.Code != http.StatusOK {
		t.Fatalf("replay status = %d", replay.Code)
	}

	conflict := post(server, "/payments", []byte(`{"payment_id":"6f1c7a52-9c21-4c0e-8e1c-0a9f7d1c2b11","amount_paise":1,"idempotency_key":"pay-1"}`))
	if conflict.Code != http.StatusConflict {
		t.Fatalf("conflict status = %d", conflict.Code)
	}

	paid := post(server, "/payments/6f1c7a52-9c21-4c0e-8e1c-0a9f7d1c2b11/succeed", nil)
	if paid.Code != http.StatusOK {
		t.Fatalf("succeed status = %d body=%s", paid.Code, paid.Body.String())
	}
	again := post(server, "/payments/6f1c7a52-9c21-4c0e-8e1c-0a9f7d1c2b11/replay-webhook", nil)
	if again.Code != http.StatusOK || len(deliveries) != 2 || !bytes.Equal(deliveries[0], deliveries[1]) {
		t.Fatalf("deliveries = %d", len(deliveries))
	}
}

func TestFailSendsFailedWebhook(t *testing.T) {
	t.Parallel()

	var status string
	webhook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if bytes.Contains(body, []byte(`"status":"failed"`)) {
			status = "failed"
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(webhook.Close)

	server := NewServer(NewStore("secret"), webhook.URL, "secret", nil)
	post(server, "/payments", []byte(`{"payment_id":"7a2d8c63-2e6e-49a6-8732-0a127349a501","amount_paise":5000,"idempotency_key":"pay-2"}`))
	failed := post(server, "/payments/7a2d8c63-2e6e-49a6-8732-0a127349a501/fail", nil)
	if failed.Code != http.StatusOK || status != "failed" {
		t.Fatalf("fail status = %d webhook=%s", failed.Code, status)
	}
}

func post(server *Server, path string, body []byte) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, request)
	return recorder
}
