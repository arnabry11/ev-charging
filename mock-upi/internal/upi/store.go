package upi

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sync"
	"time"

	"github.com/google/uuid"
)

const (
	StatusPending = "pending"
	StatusPaid    = "paid"
	StatusFailed  = "failed"
)

type Payment struct {
	ID             string `json:"payment_id"`
	AmountPaise    int64  `json:"amount_paise"`
	IdempotencyKey string `json:"idempotency_key,omitempty"`
	Status         string `json:"status"`
}

type webhook struct {
	EventID     string `json:"event_id"`
	PaymentID   string `json:"payment_id"`
	Status      string `json:"status"`
	AmountPaise int64  `json:"amount_paise"`
}

type storedPayment struct {
	Payment
	LastBody []byte
}

type Store struct {
	mu     sync.Mutex
	byID   map[string]*storedPayment
	byKey  map[string]string
	now    func() time.Time
	newID  func() string
	secret []byte
}

func NewStore(secret string) *Store {
	return &Store{
		byID:   map[string]*storedPayment{},
		byKey:  map[string]string{},
		now:    time.Now,
		newID:  uuid.NewString,
		secret: []byte(secret),
	}
}

type CreateResult struct {
	Payment  Payment
	Created  bool
	Conflict bool
}

func (s *Store) Create(id, idempotencyKey string, amount int64) CreateResult {
	s.mu.Lock()
	defer s.mu.Unlock()

	if existingID, ok := s.byKey[idempotencyKey]; ok {
		existing := s.byID[existingID]
		if existing.ID == id && existing.AmountPaise == amount {
			return CreateResult{Payment: existing.Payment}
		}
		return CreateResult{Conflict: true}
	}
	if _, ok := s.byID[id]; ok {
		return CreateResult{Conflict: true}
	}

	payment := &storedPayment{Payment: Payment{
		ID:             id,
		AmountPaise:    amount,
		IdempotencyKey: idempotencyKey,
		Status:         StatusPending,
	}}
	s.byID[id] = payment
	s.byKey[idempotencyKey] = id
	return CreateResult{Payment: payment.Payment, Created: true}
}

func (s *Store) Mark(id, status string) (Payment, []byte, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	payment, ok := s.byID[id]
	if !ok || payment.Status != StatusPending {
		if !ok {
			return Payment{}, nil, false
		}
		return payment.Payment, nil, false
	}

	payment.Status = status
	body, err := json.Marshal(webhook{
		EventID:     s.newID(),
		PaymentID:   payment.ID,
		Status:      status,
		AmountPaise: payment.AmountPaise,
	})
	if err != nil {
		return Payment{}, nil, false
	}
	payment.LastBody = body
	return payment.Payment, body, true
}

func (s *Store) RevertPending(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	payment, ok := s.byID[id]
	if !ok {
		return
	}
	payment.Status = StatusPending
	payment.LastBody = nil
}

func (s *Store) LastWebhook(id string) ([]byte, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	payment, ok := s.byID[id]
	if !ok || len(payment.LastBody) == 0 {
		return nil, false
	}
	return payment.LastBody, true
}

func (s *Store) Timestamp() string {
	return s.now().UTC().Format(time.RFC3339)
}

func Sign(secret []byte, timestamp string, body []byte) string {
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(timestamp))
	_, _ = mac.Write([]byte("."))
	_, _ = mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}
