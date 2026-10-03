package session

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/arnabry11/ev-charging/gateway/internal/registry"
	"github.com/arnabry11/ev-charging/gateway/internal/store"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestStartIsIdempotent(t *testing.T) {
	t.Parallel()

	repo := newFakeRepository()
	chargers := registry.New()
	chargers.Connected("CHG-1")
	chargers.ConnectorStatus("CHG-1", registry.Connector{
		ID:        1,
		Status:    "Available",
		ErrorCode: "NoError",
	}, time.Now())
	commands := &fakeCommander{startAccepted: true}
	engine := New(repo, chargers, commands)
	engine.now = func() time.Time { return time.Date(2026, time.October, 3, 8, 0, 0, 0, time.UTC) }
	request := validStartRequest()

	first, err := engine.Start(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	second, err := engine.Start(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}

	if first.Status != http.StatusAccepted || first.Result.State != StateStartRequested {
		t.Fatalf("first response = %+v", first)
	}
	if second != first {
		t.Fatalf("replay = %+v, want %+v", second, first)
	}
	if commands.startCalls != 1 {
		t.Fatalf("remote start calls = %d", commands.startCalls)
	}
	if len(repo.sessions) != 1 {
		t.Fatalf("sessions = %d", len(repo.sessions))
	}
}

func TestStartRejectsReusedCommandID(t *testing.T) {
	t.Parallel()

	repo := newFakeRepository()
	chargers := registry.New()
	commands := &fakeCommander{}
	engine := New(repo, chargers, commands)
	engine.now = func() time.Time { return time.Date(2026, time.October, 3, 8, 0, 0, 0, time.UTC) }

	first := validStartRequest()
	if _, err := engine.Start(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	second := first
	second.SessionRef = uuid.New()
	response, err := engine.Start(context.Background(), second)
	if err != nil {
		t.Fatal(err)
	}

	if response.Status != http.StatusConflict || response.Result.Result != "command_id_reused" {
		t.Fatalf("response = %+v", response)
	}
}

func TestStartRejectsUnavailableConnectorWithoutCallingCharger(t *testing.T) {
	t.Parallel()

	repo := newFakeRepository()
	chargers := registry.New()
	chargers.Connected("CHG-1")
	commands := &fakeCommander{}
	engine := New(repo, chargers, commands)
	engine.now = func() time.Time { return time.Date(2026, time.October, 3, 8, 0, 0, 0, time.UTC) }

	response, err := engine.Start(context.Background(), validStartRequest())
	if err != nil {
		t.Fatal(err)
	}

	if response.Status != http.StatusConflict || response.Result.Result != "connector_busy" {
		t.Fatalf("response = %+v", response)
	}
	if commands.startCalls != 0 {
		t.Fatalf("remote start calls = %d", commands.startCalls)
	}
}

func TestStopIsIdempotent(t *testing.T) {
	t.Parallel()

	repo := newFakeRepository()
	sessionRef := uuid.MustParse("b3a0f1d2-1111-4444-8888-123456789abc")
	repo.sessions[sessionRef] = store.Session{
		SessionRef:        uuidType(sessionRef),
		ChargerID:         "CHG-1",
		State:             StateActive,
		OcppTransactionID: pgtype.Int4{Int32: 42, Valid: true},
	}
	commands := &fakeCommander{stopAccepted: true}
	engine := New(repo, registry.New(), commands)
	request := StopRequest{CommandID: uuid.New(), SessionRef: sessionRef}

	first, err := engine.Stop(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	second, err := engine.Stop(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}

	if first.Status != http.StatusAccepted || first.Result.State != StateStopping {
		t.Fatalf("first response = %+v", first)
	}
	if second != first {
		t.Fatalf("replay = %+v, want %+v", second, first)
	}
	if commands.stopCalls != 1 {
		t.Fatalf("remote stop calls = %d", commands.stopCalls)
	}
}

func TestCompletedStartReplayIgnoresExpiry(t *testing.T) {
	t.Parallel()

	repo := newFakeRepository()
	chargers := registry.New()
	chargers.Connected("CHG-1")
	chargers.ConnectorStatus("CHG-1", registry.Connector{ID: 1, Status: "Available"}, time.Now())
	engine := New(repo, chargers, &fakeCommander{startAccepted: true})
	now := time.Date(2026, time.October, 3, 8, 0, 0, 0, time.UTC)
	engine.now = func() time.Time { return now }
	request := validStartRequest()

	first, err := engine.Start(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	now = request.ExpiresAt.Add(time.Minute)
	replayed, err := engine.Start(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if replayed != first {
		t.Fatalf("replay = %+v, want %+v", replayed, first)
	}
}

func TestStartTimeoutLeavesSessionRecoverable(t *testing.T) {
	t.Parallel()

	repo := newFakeRepository()
	chargers := registry.New()
	chargers.Connected("CHG-1")
	chargers.ConnectorStatus("CHG-1", registry.Connector{ID: 1, Status: "Available"}, time.Now())
	commands := &fakeCommander{startErr: context.DeadlineExceeded}
	engine := New(repo, chargers, commands)
	now := time.Date(2026, time.October, 3, 8, 0, 0, 0, time.UTC)
	engine.now = func() time.Time { return now }
	request := validStartRequest()

	response, err := engine.Start(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if response.Result.State != StateStartRequested || response.Result.Result != "processing" {
		t.Fatalf("response = %+v", response)
	}
	if repo.commands[request.CommandID].HttpStatus.Valid {
		t.Fatal("ambiguous command must remain resumable")
	}

	transactionID, err := engine.Activate(context.Background(), request.ChargerID, int(request.ConnectorID), request.IDTag, 1_000, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	replayedID, err := engine.Activate(context.Background(), request.ChargerID, int(request.ConnectorID), request.IDTag, 1_000, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if replayedID != transactionID {
		t.Fatalf("replayed transaction ID = %d, want %d", replayedID, transactionID)
	}

	now = request.ExpiresAt.Add(time.Minute)
	recovered, err := engine.Start(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if recovered.Result.Result != "accepted" {
		t.Fatalf("recovered response = %+v", recovered)
	}
	if commands.startCalls != 1 {
		t.Fatalf("remote start calls = %d, want 1", commands.startCalls)
	}
}

func TestStartQueueTimeoutRemainsRetryable(t *testing.T) {
	t.Parallel()

	repo := newFakeRepository()
	chargers := registry.New()
	chargers.Connected("CHG-1")
	chargers.ConnectorStatus("CHG-1", registry.Connector{ID: 1, Status: "Available"}, time.Now())
	commands := &fakeCommander{startErr: context.DeadlineExceeded, skipBeforeSend: true}
	engine := New(repo, chargers, commands)
	engine.now = func() time.Time { return time.Date(2026, time.October, 3, 8, 0, 0, 0, time.UTC) }
	request := validStartRequest()

	response, err := engine.Start(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if response.Result.Result != "processing" {
		t.Fatalf("response = %+v", response)
	}
	if repo.commands[request.CommandID].State != "received" {
		t.Fatalf("command state = %s, want received", repo.commands[request.CommandID].State)
	}
}

func TestStartReconcilesAmbiguousDispatchTransition(t *testing.T) {
	t.Parallel()

	repo := newFakeRepository()
	repo.markDispatchErr = context.DeadlineExceeded
	repo.markDispatchCommits = true
	chargers := registry.New()
	chargers.Connected("CHG-1")
	chargers.ConnectorStatus("CHG-1", registry.Connector{ID: 1, Status: "Available"}, time.Now())
	engine := New(repo, chargers, &fakeCommander{startAccepted: true})
	engine.now = func() time.Time { return time.Date(2026, time.October, 3, 8, 0, 0, 0, time.UTC) }
	request := validStartRequest()

	response, err := engine.Start(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if response.Result.Result != "accepted" {
		t.Fatalf("response = %+v", response)
	}
}

func TestExpiredRecoveredStartReleasesConnector(t *testing.T) {
	t.Parallel()

	repo := newFakeRepository()
	request := validStartRequest()
	raw, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.InsertCommand(context.Background(), store.InsertCommandParams{
		CommandID:   uuidType(request.CommandID),
		CommandType: CommandStart,
		Request:     raw,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CreateSession(context.Background(), store.CreateSessionParams{
		SessionRef:     uuidType(request.SessionRef),
		StartCommandID: uuidType(request.CommandID),
		ChargerID:      request.ChargerID,
		ConnectorID:    request.ConnectorID,
		IDTag:          request.IDTag,
		LimitEnergyWh:  request.Limits.MaxEnergyWh,
		LimitDurationS: request.Limits.MaxDurationS,
	}); err != nil {
		t.Fatal(err)
	}

	engine := New(repo, registry.New(), &fakeCommander{})
	engine.now = func() time.Time { return request.ExpiresAt.Add(time.Minute) }
	response, err := engine.Start(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if response.Result.State != "invalid" {
		t.Fatalf("response = %+v", response)
	}
	if repo.sessions[request.SessionRef].State != StateFailed {
		t.Fatalf("session state = %s", repo.sessions[request.SessionRef].State)
	}
}

func TestFinishIsIdempotentAndScopedToCharger(t *testing.T) {
	t.Parallel()

	repo := newFakeRepository()
	ref := uuid.New()
	repo.sessions[ref] = store.Session{
		SessionRef:        uuidType(ref),
		ChargerID:         "CHG-1",
		State:             StateActive,
		OcppTransactionID: pgtype.Int4{Int32: 42, Valid: true},
	}
	engine := New(repo, registry.New(), &fakeCommander{})

	if err := engine.Finish(context.Background(), "CHG-2", 42, 2_000, time.Now(), "Remote"); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("cross-charger finish error = %v", err)
	}
	if err := engine.Finish(context.Background(), "CHG-1", 42, 2_000, time.Now(), "Remote"); err != nil {
		t.Fatal(err)
	}
	if err := engine.Finish(context.Background(), "CHG-1", 42, 2_000, time.Now(), "Remote"); err != nil {
		t.Fatal(err)
	}
}

func TestStartCallbackWinsConfirmationRace(t *testing.T) {
	t.Parallel()

	repo := newFakeRepository()
	chargers := registry.New()
	chargers.Connected("CHG-1")
	chargers.ConnectorStatus("CHG-1", registry.Connector{ID: 1, Status: "Available"}, time.Now())
	commands := &fakeCommander{}
	engine := New(repo, chargers, commands)
	engine.now = func() time.Time { return time.Date(2026, time.October, 3, 8, 0, 0, 0, time.UTC) }
	commands.startHook = func(chargerID string, connectorID int, idTag string) {
		if _, err := engine.Activate(context.Background(), chargerID, connectorID, idTag, 1_000, time.Now()); err != nil {
			t.Error(err)
		}
	}

	response, err := engine.Start(context.Background(), validStartRequest())
	if err != nil {
		t.Fatal(err)
	}
	if response.Result.Result != "accepted" {
		t.Fatalf("response = %+v", response)
	}
}

func TestStartFinalizesAfterRequestCancellation(t *testing.T) {
	t.Parallel()

	repo := newFakeRepository()
	chargers := registry.New()
	chargers.Connected("CHG-1")
	chargers.ConnectorStatus("CHG-1", registry.Connector{ID: 1, Status: "Available"}, time.Now())
	ctx, cancel := context.WithCancel(context.Background())
	commands := &fakeCommander{startAccepted: true, startHook: func(string, int, string) { cancel() }}
	engine := New(repo, chargers, commands)
	engine.now = func() time.Time { return time.Date(2026, time.October, 3, 8, 0, 0, 0, time.UTC) }
	request := validStartRequest()

	response, err := engine.Start(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if response.Result.Result != "accepted" {
		t.Fatalf("response = %+v", response)
	}
	if repo.commands[request.CommandID].State != "completed" {
		t.Fatalf("command state = %s", repo.commands[request.CommandID].State)
	}
}

func TestStopCallbackWinsConfirmationRace(t *testing.T) {
	t.Parallel()

	repo := newFakeRepository()
	sessionRef := uuid.New()
	repo.sessions[sessionRef] = store.Session{
		SessionRef:        uuidType(sessionRef),
		ChargerID:         "CHG-1",
		State:             StateActive,
		OcppTransactionID: pgtype.Int4{Int32: 42, Valid: true},
	}
	commands := &fakeCommander{}
	engine := New(repo, registry.New(), commands)
	commands.stopHook = func(chargerID string, transactionID int) {
		if err := engine.Finish(context.Background(), chargerID, transactionID, 2_000, time.Now(), "Remote"); err != nil {
			t.Error(err)
		}
	}

	response, err := engine.Stop(context.Background(), StopRequest{CommandID: uuid.New(), SessionRef: sessionRef})
	if err != nil {
		t.Fatal(err)
	}
	if response.Result.Result != "accepted" {
		t.Fatalf("response = %+v", response)
	}
	if repo.sessions[sessionRef].State != StateStopped {
		t.Fatalf("session state = %s", repo.sessions[sessionRef].State)
	}
}

func validStartRequest() StartRequest {
	return StartRequest{
		CommandID:   uuid.MustParse("6f1c7a52-9c21-4c0e-8e1c-0a9f7d1c2b11"),
		SessionRef:  uuid.MustParse("b3a0f1d2-1111-4444-8888-123456789abc"),
		ChargerID:   "CHG-1",
		ConnectorID: 1,
		IDTag:       "S-b3a0f1d2",
		Limits:      Limits{MaxEnergyWh: 12_000, MaxDurationS: 5_400},
		ExpiresAt:   time.Date(2026, time.October, 3, 8, 5, 0, 0, time.UTC),
	}
}

type fakeCommander struct {
	startAccepted  bool
	stopAccepted   bool
	startErr       error
	stopErr        error
	startHook      func(string, int, string)
	stopHook       func(string, int)
	skipBeforeSend bool
	startCalls     int
	stopCalls      int
}

func (f *fakeCommander) RemoteStart(_ context.Context, chargerID string, connectorID int, idTag string, beforeSend func() error) (bool, error) {
	f.startCalls++
	if !f.skipBeforeSend {
		if err := beforeSend(); err != nil {
			return false, err
		}
	}
	if f.startHook != nil {
		f.startHook(chargerID, connectorID, idTag)
	}
	return f.startAccepted, f.startErr
}

func (f *fakeCommander) RemoteStop(_ context.Context, chargerID string, transactionID int, beforeSend func() error) (bool, error) {
	f.stopCalls++
	if !f.skipBeforeSend {
		if err := beforeSend(); err != nil {
			return false, err
		}
	}
	if f.stopHook != nil {
		f.stopHook(chargerID, transactionID)
	}
	return f.stopAccepted, f.stopErr
}

type fakeRepository struct {
	commands            map[uuid.UUID]store.CommandInbox
	sessions            map[uuid.UUID]store.Session
	markDispatchErr     error
	markDispatchCommits bool
}

func newFakeRepository() *fakeRepository {
	return &fakeRepository{
		commands: make(map[uuid.UUID]store.CommandInbox),
		sessions: make(map[uuid.UUID]store.Session),
	}
}

func (f *fakeRepository) InsertCommand(_ context.Context, arg store.InsertCommandParams) (int64, error) {
	id := uuid.UUID(arg.CommandID.Bytes)
	if _, ok := f.commands[id]; ok {
		return 0, nil
	}
	f.commands[id] = store.CommandInbox{
		CommandID:   arg.CommandID,
		CommandType: arg.CommandType,
		Request:     arg.Request,
		State:       "received",
	}
	return 1, nil
}

func (f *fakeRepository) GetCommand(_ context.Context, id pgtype.UUID) (store.CommandInbox, error) {
	command, ok := f.commands[uuid.UUID(id.Bytes)]
	if !ok {
		return store.CommandInbox{}, pgx.ErrNoRows
	}
	return command, nil
}

func (f *fakeRepository) ClaimCommand(_ context.Context, arg store.ClaimCommandParams) (store.CommandInbox, error) {
	command, ok := f.commands[uuid.UUID(arg.CommandID.Bytes)]
	if !ok || command.HttpStatus.Valid || command.State != "received" {
		return store.CommandInbox{}, pgx.ErrNoRows
	}
	command.ClaimToken = arg.ClaimToken
	f.commands[uuid.UUID(arg.CommandID.Bytes)] = command
	return command, nil
}

func (f *fakeRepository) MarkCommandDispatching(_ context.Context, arg store.MarkCommandDispatchingParams) (store.CommandInbox, error) {
	command, ok := f.commands[uuid.UUID(arg.CommandID.Bytes)]
	if !ok || command.State != "received" || command.ClaimToken != arg.ClaimToken {
		return store.CommandInbox{}, pgx.ErrNoRows
	}
	command.State = "dispatching"
	f.commands[uuid.UUID(arg.CommandID.Bytes)] = command
	if f.markDispatchErr != nil && f.markDispatchCommits {
		return store.CommandInbox{}, f.markDispatchErr
	}
	if f.markDispatchErr != nil {
		command.State = "received"
		f.commands[uuid.UUID(arg.CommandID.Bytes)] = command
		return store.CommandInbox{}, f.markDispatchErr
	}
	return command, nil
}

func (f *fakeRepository) CompleteCommand(_ context.Context, arg store.CompleteCommandParams) (int64, error) {
	id := uuid.UUID(arg.CommandID.Bytes)
	command := f.commands[id]
	if command.State == "completed" || command.ClaimToken != arg.ClaimToken {
		return 0, nil
	}
	command.Result = arg.Result
	command.HttpStatus = arg.HttpStatus
	command.State = "completed"
	command.ClaimToken = pgtype.UUID{}
	f.commands[id] = command
	return 1, nil
}

func (f *fakeRepository) CreateSession(_ context.Context, arg store.CreateSessionParams) (store.Session, error) {
	id := uuid.UUID(arg.SessionRef.Bytes)
	if _, ok := f.sessions[id]; ok {
		return store.Session{}, errors.New("duplicate session")
	}
	value := store.Session{
		SessionRef:     arg.SessionRef,
		StartCommandID: arg.StartCommandID,
		ChargerID:      arg.ChargerID,
		ConnectorID:    arg.ConnectorID,
		IDTag:          arg.IDTag,
		State:          StateStartRequested,
		LimitEnergyWh:  arg.LimitEnergyWh,
		LimitDurationS: arg.LimitDurationS,
	}
	f.sessions[id] = value
	return value, nil
}

func (f *fakeRepository) GetSession(_ context.Context, id pgtype.UUID) (store.Session, error) {
	value, ok := f.sessions[uuid.UUID(id.Bytes)]
	if !ok {
		return store.Session{}, pgx.ErrNoRows
	}
	return value, nil
}

func (f *fakeRepository) GetSessionForStart(_ context.Context, arg store.GetSessionForStartParams) (store.Session, error) {
	for _, value := range f.sessions {
		if value.ChargerID == arg.ChargerID && value.ConnectorID == arg.ConnectorID &&
			value.IDTag == arg.IDTag && (value.State == StateStartRequested || value.State == StateActive) {
			return value, nil
		}
	}
	return store.Session{}, pgx.ErrNoRows
}

func (f *fakeRepository) ActivateSession(_ context.Context, arg store.ActivateSessionParams) (store.Session, error) {
	id := uuid.UUID(arg.SessionRef.Bytes)
	value, ok := f.sessions[id]
	if !ok || value.State != StateStartRequested {
		return store.Session{}, pgx.ErrNoRows
	}
	value.State = StateActive
	value.OcppTransactionID = pgtype.Int4{Int32: 42, Valid: true}
	value.MeterStartWh = arg.MeterStartWh
	value.StartedAt = arg.StartedAt
	f.sessions[id] = value
	return value, nil
}

func (f *fakeRepository) MarkSessionStopping(_ context.Context, arg store.MarkSessionStoppingParams) (store.Session, error) {
	id := uuid.UUID(arg.SessionRef.Bytes)
	value, ok := f.sessions[id]
	if !ok {
		return store.Session{}, pgx.ErrNoRows
	}
	value.State = StateStopping
	value.StopSource = arg.StopSource
	value.StopCommandID = arg.StopCommandID
	f.sessions[id] = value
	return value, nil
}

func (f *fakeRepository) RestoreSessionActive(_ context.Context, id pgtype.UUID) (int64, error) {
	value, ok := f.sessions[uuid.UUID(id.Bytes)]
	if ok && value.State == StateStopping {
		value.State = StateActive
		value.StopCommandID = pgtype.UUID{}
		f.sessions[uuid.UUID(id.Bytes)] = value
		return 1, nil
	}
	return 0, nil
}

func (f *fakeRepository) FailSession(_ context.Context, arg store.FailSessionParams) (int64, error) {
	value, ok := f.sessions[uuid.UUID(arg.SessionRef.Bytes)]
	if !ok || value.State != StateStartRequested {
		return 0, nil
	}
	value.State = StateFailed
	value.StopReason = arg.StopReason
	f.sessions[uuid.UUID(arg.SessionRef.Bytes)] = value
	return 1, nil
}

func (f *fakeRepository) GetSessionByTransaction(_ context.Context, arg store.GetSessionByTransactionParams) (store.Session, error) {
	for _, value := range f.sessions {
		if value.ChargerID == arg.ChargerID && value.OcppTransactionID == arg.OcppTransactionID {
			return value, nil
		}
	}
	return store.Session{}, pgx.ErrNoRows
}

func (f *fakeRepository) StopSession(_ context.Context, arg store.StopSessionParams) (store.Session, error) {
	for id, value := range f.sessions {
		if value.ChargerID == arg.ChargerID && value.OcppTransactionID == arg.OcppTransactionID &&
			(value.State == StateActive || value.State == StateStopping) {
			value.State = StateStopped
			value.MeterStopWh = arg.MeterStopWh
			value.StoppedAt = arg.StoppedAt
			f.sessions[id] = value
			return value, nil
		}
	}
	return store.Session{}, pgx.ErrNoRows
}
