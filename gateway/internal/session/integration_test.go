package session

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/arnabry11/ev-charging/gateway/internal/store"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestTransactionCallbacksAreConcurrentSafe(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if _, err := pool.Exec(ctx, "TRUNCATE sessions, command_inbox"); err != nil {
		t.Fatal(err)
	}

	queries := store.New(pool)
	commandID := uuid.New()
	if _, err := queries.InsertCommand(ctx, store.InsertCommandParams{
		CommandID:   uuidType(commandID),
		CommandType: CommandStart,
		Request:     []byte(`{"session_ref":"one"}`),
	}); err != nil {
		t.Fatal(err)
	}
	sessionRef := uuid.New()
	if _, err := queries.CreateSession(ctx, store.CreateSessionParams{
		SessionRef:     uuidType(sessionRef),
		StartCommandID: uuidType(commandID),
		ChargerID:      "CHG-1",
		ConnectorID:    1,
		IDTag:          "ID-1",
		LimitEnergyWh:  10_000,
		LimitDurationS: 3_600,
	}); err != nil {
		t.Fatal(err)
	}

	engine := New(queries, nil, nil)
	transactionIDs := runOperations(t, 12, func() (int, error) {
		return engine.Activate(ctx, "CHG-1", 1, "ID-1", 1_000, time.Now())
	})
	for _, transactionID := range transactionIDs {
		if transactionID != transactionIDs[0] {
			t.Fatalf("transaction IDs = %v", transactionIDs)
		}
	}

	if err := engine.Meter(ctx, "CHG-1", transactionIDs[0], 2_500, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := engine.Finish(ctx, "CHG-1", transactionIDs[0], 2_499, time.Now(), "Remote"); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("regressing stop meter error = %v, want no rows", err)
	}
	runOperations(t, 12, func() (struct{}, error) {
		return struct{}{}, engine.Finish(ctx, "CHG-1", transactionIDs[0], 2_500, time.Now(), "Remote")
	})
}

func runOperations[T any](t *testing.T, count int, operation func() (T, error)) []T {
	t.Helper()

	results := make(chan T, count)
	errs := make(chan error, count)
	var group sync.WaitGroup
	for range count {
		group.Add(1)
		go func() {
			defer group.Done()
			result, err := operation()
			results <- result
			errs <- err
		}()
	}
	group.Wait()
	close(results)
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	return channelValues(results)
}

func channelValues[T any](values <-chan T) []T {
	var result []T
	for value := range values {
		result = append(result, value)
	}
	return result
}
