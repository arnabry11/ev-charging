package upi

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
)

type Server struct {
	store      *Store
	webhookURL string
	secret     []byte
	client     *http.Client
	logger     *slog.Logger
}

func NewServer(store *Store, webhookURL, secret string, logger *slog.Logger) *Server {
	if logger == nil {
		logger = slog.Default()
	}
	return &Server{
		store:      store,
		webhookURL: webhookURL,
		secret:     []byte(secret),
		client:     &http.Client{Timeout: 5 * time.Second},
		logger:     logger,
	}
}

func (s *Server) Handler() http.Handler {
	router := chi.NewRouter()
	router.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	router.Post("/payments", s.create)
	router.Post("/payments/{paymentID}/succeed", s.mark(StatusPaid))
	router.Post("/payments/{paymentID}/fail", s.mark(StatusFailed))
	router.Post("/payments/{paymentID}/replay-webhook", s.replay)
	return router
}

func (s *Server) create(w http.ResponseWriter, r *http.Request) {
	var request struct {
		PaymentID      string `json:"payment_id"`
		AmountPaise    int64  `json:"amount_paise"`
		IdempotencyKey string `json:"idempotency_key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil || request.PaymentID == "" || request.IdempotencyKey == "" || request.AmountPaise <= 0 {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "invalid_payment"})
		return
	}

	result := s.store.Create(request.PaymentID, request.IdempotencyKey, request.AmountPaise)
	if result.Conflict {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "idempotency_conflict"})
		return
	}
	status := http.StatusOK
	if result.Created {
		status = http.StatusCreated
	}
	writeJSON(w, status, result.Payment)
}

func (s *Server) mark(status string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		payment, body, ok := s.store.Mark(chi.URLParam(r, "paymentID"), status)
		if payment.ID == "" {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "payment_not_found"})
			return
		}
		if !ok {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "payment_not_pending"})
			return
		}
		if err := s.deliver(body); err != nil {
			s.store.RevertPending(payment.ID)
			s.logger.Error("upi webhook", "err", err, "payment_id", payment.ID)
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "webhook_failed"})
			return
		}
		writeJSON(w, http.StatusOK, payment)
	}
}

func (s *Server) replay(w http.ResponseWriter, r *http.Request) {
	body, ok := s.store.LastWebhook(chi.URLParam(r, "paymentID"))
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "webhook_not_found"})
		return
	}
	if err := s.deliver(body); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "webhook_failed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "replayed"})
}

func (s *Server) deliver(body []byte) error {
	if s.webhookURL == "" {
		return nil
	}
	timestamp := s.store.Timestamp()
	request, err := http.NewRequest(http.MethodPost, s.webhookURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Timestamp", timestamp)
	request.Header.Set("X-Signature", Sign(s.secret, timestamp, body))
	response, err := s.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return errStatus(response.StatusCode)
	}
	return nil
}

type statusError int

func (e statusError) Error() string {
	return http.StatusText(int(e))
}

func errStatus(code int) error {
	return statusError(code)
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
