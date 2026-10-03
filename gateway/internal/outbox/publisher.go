package outbox

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/arnabry11/ev-charging/gateway/internal/signing"
	"github.com/arnabry11/ev-charging/gateway/internal/store"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

type repository interface {
	ClaimOutboxBatch(context.Context, int32) ([]store.Outbox, error)
	MarkOutboxPublished(context.Context, pgtype.UUID) (int64, error)
	ReleaseOutbox(context.Context, pgtype.UUID) error
}

type Publisher struct {
	repo      repository
	url       string
	secret    []byte
	client    *http.Client
	logger    *slog.Logger
	batchSize int32
	interval  time.Duration
	now       func() time.Time
}

func New(repo repository, url, secret string, logger *slog.Logger) *Publisher {
	if logger == nil {
		logger = slog.Default()
	}
	return &Publisher{
		repo:      repo,
		url:       url,
		secret:    []byte(secret),
		client:    &http.Client{Timeout: 5 * time.Second},
		logger:    logger,
		batchSize: 20,
		interval:  200 * time.Millisecond,
		now:       time.Now,
	}
}

func (p *Publisher) Run(ctx context.Context) {
	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()
	for {
		if err := p.Publish(ctx); err != nil {
			p.logger.Error("publish outbox", "err", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (p *Publisher) Publish(ctx context.Context) error {
	events, err := p.repo.ClaimOutboxBatch(ctx, p.batchSize)
	if err != nil {
		return err
	}
	for _, event := range events {
		if err := p.publishOne(ctx, event); err != nil {
			_ = p.repo.ReleaseOutbox(ctx, event.EventID)
			p.logger.Error("gateway event delivery", "event_id", uuid.UUID(event.EventID.Bytes).String(), "err", err)
			continue
		}
		if _, err := p.repo.MarkOutboxPublished(ctx, event.EventID); err != nil {
			return err
		}
	}
	return nil
}

func (p *Publisher) publishOne(ctx context.Context, event store.Outbox) error {
	body, err := json.Marshal(envelope{
		EventID:    uuid.UUID(event.EventID.Bytes).String(),
		EventType:  event.EventType,
		SessionRef: uuid.UUID(event.SessionRef.Bytes).String(),
		Sequence:   event.Sequence,
		OccurredAt: event.OccurredAt.Time.UTC().Format(time.RFC3339),
		Payload:    event.Payload,
	})
	if err != nil {
		return err
	}
	timestamp := p.now().UTC().Format(time.RFC3339)
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, p.url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set(signing.TimestampHeader, timestamp)
	request.Header.Set(signing.SignatureHeader, signing.Sign(p.secret, timestamp, body))

	response, err := p.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	responseBody, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("platform returned %d: %s", response.StatusCode, responseBody)
	}
	return nil
}

type envelope struct {
	EventID    string          `json:"event_id"`
	EventType  string          `json:"event_type"`
	SessionRef string          `json:"session_ref"`
	Sequence   int64           `json:"sequence"`
	OccurredAt string          `json:"occurred_at"`
	Payload    json.RawMessage `json:"payload"`
}
