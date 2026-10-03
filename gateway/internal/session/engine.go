package session

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"sync"
	"time"

	"github.com/arnabry11/ev-charging/gateway/internal/registry"
	"github.com/arnabry11/ev-charging/gateway/internal/store"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

const (
	CommandStart = "start_session"
	CommandStop  = "stop_session"

	StateStartRequested = "start_requested"
	StateActive         = "active"
	StateStopping       = "stopping"
	StateStopped        = "stopped"
	StateFailed         = "failed"

	commandWorkTimeout = 30 * time.Second
	limitRetryDelay    = 5 * time.Second
)

var errDispatchNotStarted = errors.New("command dispatch not started")

type Limits struct {
	MaxEnergyWh  int64 `json:"max_energy_wh"`
	MaxDurationS int32 `json:"max_duration_s"`
}

type StartRequest struct {
	CommandID   uuid.UUID `json:"command_id"`
	SessionRef  uuid.UUID `json:"session_ref"`
	ChargerID   string    `json:"charger_id"`
	ConnectorID int32     `json:"connector_id"`
	IDTag       string    `json:"id_tag"`
	Limits      Limits    `json:"limits"`
	ExpiresAt   time.Time `json:"expires_at"`
}

type StopRequest struct {
	CommandID  uuid.UUID `json:"command_id"`
	SessionRef uuid.UUID `json:"session_ref"`
}

type Result struct {
	State  string `json:"state"`
	Result string `json:"result,omitempty"`
	Detail string `json:"detail,omitempty"`
}

type Response struct {
	Status int
	Result Result
}

type repository interface {
	InsertCommand(context.Context, store.InsertCommandParams) (int64, error)
	GetCommand(context.Context, pgtype.UUID) (store.CommandInbox, error)
	ClaimCommand(context.Context, store.ClaimCommandParams) (store.CommandInbox, error)
	MarkCommandDispatching(context.Context, store.MarkCommandDispatchingParams) (store.CommandInbox, error)
	CompleteCommand(context.Context, store.CompleteCommandParams) (int64, error)
	CreateSession(context.Context, store.CreateSessionParams) (store.Session, error)
	GetSession(context.Context, pgtype.UUID) (store.Session, error)
	ListRecoverableSessions(context.Context) ([]store.Session, error)
	GetSessionForStart(context.Context, store.GetSessionForStartParams) (store.Session, error)
	ActivateSession(context.Context, store.ActivateSessionParams) (store.Session, error)
	MarkSessionStopping(context.Context, store.MarkSessionStoppingParams) (store.Session, error)
	RestoreSessionActive(context.Context, pgtype.UUID) (int64, error)
	RestoreLimitSessionActive(context.Context, store.RestoreLimitSessionActiveParams) (int64, error)
	FailSession(context.Context, store.FailSessionParams) (int64, error)
	GetSessionByTransaction(context.Context, store.GetSessionByTransactionParams) (store.Session, error)
	RecordMeterValue(context.Context, store.RecordMeterValueParams) (store.Session, error)
	StopSession(context.Context, store.StopSessionParams) (store.Session, error)
}

type chargerLookup interface {
	Get(id string) (registry.Charger, bool)
}

type commander interface {
	RemoteStart(context.Context, string, int, string, func() error) (bool, error)
	RemoteStop(context.Context, string, int, func() error) (bool, error)
}

type Engine struct {
	repo     repository
	chargers chargerLookup
	commands commander
	now      func() time.Time
	after    func(time.Duration) <-chan time.Time

	timersMu sync.Mutex
	timers   map[uuid.UUID]durationTimer
}

type durationTimer struct {
	token  uuid.UUID
	cancel context.CancelFunc
}

func New(repo repository, chargers chargerLookup, commands commander) *Engine {
	return &Engine{
		repo:     repo,
		chargers: chargers,
		commands: commands,
		now:      time.Now,
		after:    time.After,
		timers:   make(map[uuid.UUID]durationTimer),
	}
}

func (e *Engine) Recover(ctx context.Context) error {
	sessions, err := e.repo.ListRecoverableSessions(ctx)
	if err != nil {
		return err
	}
	for _, current := range sessions {
		if current.State == StateStopping {
			go e.dispatchLimitStop(current)
			continue
		}
		if current.MeterStartWh.Valid && current.LastEnergyWh.Valid &&
			current.LastEnergyWh.Int64-current.MeterStartWh.Int64 >= current.LimitEnergyWh {
			if err := e.requestLimitStop(ctx, current, "energy_limit"); err != nil {
				return err
			}
			continue
		}
		e.scheduleDuration(current)
	}
	return nil
}

func (e *Engine) Start(ctx context.Context, request StartRequest) (Response, error) {
	if request.CommandID == uuid.Nil {
		return invalidResponse(errors.New("command_id is required")), nil
	}

	raw, err := json.Marshal(request)
	if err != nil {
		return Response{}, err
	}
	replayed, dispatched, claimToken, response, err := e.recordCommand(ctx, request.CommandID, CommandStart, raw)
	if err != nil || replayed {
		return response, err
	}
	workCtx, cancel := commandContext(ctx)
	defer cancel()
	if dispatched {
		return e.reconcileStart(workCtx, request, claimToken)
	}

	if err := validateStart(request); err != nil {
		response = invalidResponse(err)
		return e.finalizeCommand(workCtx, request.CommandID, claimToken, response, true)
	}
	if request.ExpiresAt.IsZero() || !request.ExpiresAt.After(e.now()) {
		if err := e.failPendingStart(workCtx, request, "expired"); err != nil {
			return Response{}, err
		}
		response = invalidResponse(errors.New("command has expired"))
		return e.finalizeCommand(workCtx, request.CommandID, claimToken, response, true)
	}

	response, complete, err := e.startRecorded(workCtx, request, claimToken)
	if err != nil {
		return Response{}, err
	}
	finalizeCtx, finalizeCancel := commandContext(workCtx)
	defer finalizeCancel()
	return e.finalizeCommand(finalizeCtx, request.CommandID, claimToken, response, complete)
}

func (e *Engine) Stop(ctx context.Context, request StopRequest) (Response, error) {
	if request.CommandID == uuid.Nil {
		return invalidResponse(errors.New("command_id is required")), nil
	}
	raw, err := json.Marshal(request)
	if err != nil {
		return Response{}, err
	}
	replayed, dispatched, claimToken, response, err := e.recordCommand(ctx, request.CommandID, CommandStop, raw)
	if err != nil || replayed {
		return response, err
	}
	workCtx, cancel := commandContext(ctx)
	defer cancel()
	if dispatched {
		return e.reconcileStop(workCtx, request, claimToken)
	}

	if request.SessionRef == uuid.Nil {
		response = invalidResponse(errors.New("session_ref is required"))
		return e.finalizeCommand(workCtx, request.CommandID, claimToken, response, true)
	}

	response, complete, err := e.stopRecorded(workCtx, request, claimToken)
	if err != nil {
		return Response{}, err
	}
	finalizeCtx, finalizeCancel := commandContext(workCtx)
	defer finalizeCancel()
	return e.finalizeCommand(finalizeCtx, request.CommandID, claimToken, response, complete)
}

func (e *Engine) reconcileStart(ctx context.Context, request StartRequest, claimToken uuid.UUID) (Response, error) {
	response, complete, err := e.reconcileStartResult(ctx, request)
	if err != nil {
		return Response{}, err
	}
	return e.finalizeCommand(ctx, request.CommandID, claimToken, response, complete)
}

func (e *Engine) reconcileStartResult(ctx context.Context, request StartRequest) (Response, bool, error) {
	current, err := e.repo.GetSession(ctx, uuidType(request.SessionRef))
	if errors.Is(err, pgx.ErrNoRows) {
		return commandResponse(http.StatusAccepted, StateStartRequested, "processing"), false, nil
	}
	if err != nil {
		return Response{}, false, err
	}

	switch current.State {
	case StateActive, StateStopping, StateStopped:
		return commandResponse(http.StatusAccepted, StateStartRequested, "accepted"), true, nil
	case StateFailed:
		return commandResponse(http.StatusAccepted, StateFailed, textValue(current.StopReason, "rejected")), true, nil
	default:
		return commandResponse(http.StatusAccepted, StateStartRequested, "processing"), false, nil
	}
}

func (e *Engine) reconcileStop(ctx context.Context, request StopRequest, claimToken uuid.UUID) (Response, error) {
	response, complete, err := e.reconcileStopResult(ctx, request)
	if err != nil {
		return Response{}, err
	}
	return e.finalizeCommand(ctx, request.CommandID, claimToken, response, complete)
}

func (e *Engine) reconcileStopResult(ctx context.Context, request StopRequest) (Response, bool, error) {
	current, err := e.repo.GetSession(ctx, uuidType(request.SessionRef))
	if err != nil {
		return Response{}, false, err
	}

	switch current.State {
	case StateStopped:
		return commandResponse(http.StatusAccepted, StateStopping, "accepted"), true, nil
	case StateActive:
		return commandResponse(http.StatusAccepted, StateActive, "rejected"), true, nil
	default:
		return commandResponse(http.StatusAccepted, StateStopping, "processing"), false, nil
	}
}

func (e *Engine) Activate(ctx context.Context, chargerID string, connectorID int, idTag string, meterStartWh int64, startedAt time.Time) (int, error) {
	session, err := e.repo.GetSessionForStart(ctx, store.GetSessionForStartParams{
		ChargerID:   chargerID,
		ConnectorID: int32(connectorID),
		IDTag:       idTag,
	})
	if err != nil {
		return 0, err
	}
	if session.State == StateActive && session.OcppTransactionID.Valid {
		return int(session.OcppTransactionID.Int32), nil
	}
	active, err := e.repo.ActivateSession(ctx, store.ActivateSessionParams{
		SessionRef:   session.SessionRef,
		MeterStartWh: pgtype.Int8{Int64: meterStartWh, Valid: true},
		StartedAt:    pgtype.Timestamptz{Time: startedAt.UTC(), Valid: true},
	})
	if errors.Is(err, pgx.ErrNoRows) {
		active, err = e.repo.GetSessionForStart(ctx, store.GetSessionForStartParams{
			ChargerID:   chargerID,
			ConnectorID: int32(connectorID),
			IDTag:       idTag,
		})
	}
	if err != nil {
		return 0, err
	}
	if active.State != StateActive || !active.OcppTransactionID.Valid {
		return 0, pgx.ErrNoRows
	}
	e.scheduleDuration(active)
	return int(active.OcppTransactionID.Int32), nil
}

func (e *Engine) Meter(ctx context.Context, chargerID string, transactionID int, energyWh int64, recordedAt time.Time) error {
	ocppTransactionID := pgtype.Int4{Int32: int32(transactionID), Valid: true}
	current, err := e.repo.RecordMeterValue(ctx, store.RecordMeterValueParams{
		ChargerID:         chargerID,
		OcppTransactionID: ocppTransactionID,
		LastEnergyWh:      pgtype.Int8{Int64: energyWh, Valid: true},
	})
	if errors.Is(err, pgx.ErrNoRows) {
		existing, getErr := e.repo.GetSessionByTransaction(ctx, store.GetSessionByTransactionParams{
			ChargerID:         chargerID,
			OcppTransactionID: ocppTransactionID,
		})
		if getErr == nil && existing.LastEnergyWh.Valid && energyWh <= existing.LastEnergyWh.Int64 {
			return nil
		}
		return err
	}
	if err != nil || current.State != StateActive {
		return err
	}

	source := limitReached(current, energyWh, recordedAt)
	if source == "" {
		return nil
	}
	return e.requestLimitStop(ctx, current, source)
}

func (e *Engine) Finish(ctx context.Context, chargerID string, transactionID int, meterStopWh int64, stoppedAt time.Time, reason string) error {
	current, err := e.repo.GetSessionByTransaction(ctx, store.GetSessionByTransactionParams{
		ChargerID:         chargerID,
		OcppTransactionID: pgtype.Int4{Int32: int32(transactionID), Valid: true},
	})
	if err != nil {
		return err
	}
	if current.State == StateStopped {
		e.cancelDuration(current.SessionRef)
		return nil
	}
	_, err = e.repo.StopSession(ctx, store.StopSessionParams{
		ChargerID:         chargerID,
		OcppTransactionID: pgtype.Int4{Int32: int32(transactionID), Valid: true},
		MeterStopWh:       pgtype.Int8{Int64: meterStopWh, Valid: true},
		StoppedAt:         pgtype.Timestamptz{Time: stoppedAt.UTC(), Valid: true},
		StopReason:        pgtype.Text{String: reason, Valid: reason != ""},
	})
	if errors.Is(err, pgx.ErrNoRows) {
		current, getErr := e.repo.GetSessionByTransaction(ctx, store.GetSessionByTransactionParams{
			ChargerID:         chargerID,
			OcppTransactionID: pgtype.Int4{Int32: int32(transactionID), Valid: true},
		})
		if getErr != nil {
			return getErr
		}
		if current.State == StateStopped {
			e.cancelDuration(current.SessionRef)
			return nil
		}
		return pgx.ErrNoRows
	}
	if err == nil {
		e.cancelDuration(current.SessionRef)
	}
	return err
}

func (e *Engine) requestLimitStop(ctx context.Context, current store.Session, source string) error {
	stopping, err := e.repo.MarkSessionStopping(ctx, store.MarkSessionStoppingParams{
		SessionRef:    current.SessionRef,
		StopSource:    pgtype.Text{String: source, Valid: true},
		StopCommandID: pgtype.UUID{},
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	e.cancelDuration(current.SessionRef)
	go e.dispatchLimitStop(stopping)
	return nil
}

func (e *Engine) scheduleDuration(current store.Session) {
	if !current.StartedAt.Valid {
		return
	}
	delay := current.StartedAt.Time.Add(time.Duration(current.LimitDurationS) * time.Second).Sub(e.now())
	if delay < 0 {
		delay = 0
	}

	key := uuid.UUID(current.SessionRef.Bytes)
	token := uuid.New()
	timerCtx, cancel := context.WithCancel(context.Background())
	e.timersMu.Lock()
	if previous, ok := e.timers[key]; ok {
		previous.cancel()
	}
	e.timers[key] = durationTimer{token: token, cancel: cancel}
	e.timersMu.Unlock()

	go e.waitForDuration(timerCtx, current.SessionRef, key, token, delay)
}

func (e *Engine) waitForDuration(ctx context.Context, sessionRef pgtype.UUID, key, token uuid.UUID, delay time.Duration) {
	select {
	case <-ctx.Done():
		return
	case <-e.after(delay):
	}

	e.timersMu.Lock()
	currentTimer, ok := e.timers[key]
	if !ok || currentTimer.token != token {
		e.timersMu.Unlock()
		return
	}
	delete(e.timers, key)
	e.timersMu.Unlock()

	workCtx, cancel := context.WithTimeout(context.Background(), commandWorkTimeout)
	defer cancel()
	current, err := e.repo.GetSession(workCtx, sessionRef)
	if err != nil || current.State != StateActive {
		return
	}
	_ = e.requestLimitStop(workCtx, current, "duration_limit")
}

func (e *Engine) cancelDuration(sessionRef pgtype.UUID) {
	key := uuid.UUID(sessionRef.Bytes)
	e.timersMu.Lock()
	defer e.timersMu.Unlock()
	if timer, ok := e.timers[key]; ok {
		timer.cancel()
		delete(e.timers, key)
	}
}

func (e *Engine) dispatchLimitStop(current store.Session) {
	ctx, cancel := context.WithTimeout(context.Background(), commandWorkTimeout)
	defer cancel()
	accepted, err := e.commands.RemoteStop(
		ctx,
		current.ChargerID,
		int(current.OcppTransactionID.Int32),
		func() error { return nil },
	)
	if accepted {
		return
	}
	if isAmbiguous(err) {
		e.retryLimitStop(current)
		return
	}
	restored, restoreErr := e.repo.RestoreLimitSessionActive(ctx, store.RestoreLimitSessionActiveParams{
		SessionRef: current.SessionRef,
		StopSource: current.StopSource,
	})
	if restoreErr == nil && restored == 1 {
		e.retryLimitStop(current)
	}
}

func (e *Engine) retryLimitStop(previous store.Session) {
	go func() {
		<-e.after(limitRetryDelay)
		ctx, cancel := context.WithTimeout(context.Background(), commandWorkTimeout)
		defer cancel()
		current, err := e.repo.GetSession(ctx, previous.SessionRef)
		if err != nil {
			return
		}
		switch current.State {
		case StateActive:
			_ = e.requestLimitStop(ctx, current, previous.StopSource.String)
		case StateStopping:
			if !current.StopCommandID.Valid && current.StopSource == previous.StopSource {
				e.dispatchLimitStop(current)
			}
		}
	}()
}

func limitReached(current store.Session, energyWh int64, recordedAt time.Time) string {
	if current.MeterStartWh.Valid && energyWh-current.MeterStartWh.Int64 >= current.LimitEnergyWh {
		return "energy_limit"
	}
	if !current.StartedAt.Valid || recordedAt.Before(current.StartedAt.Time) {
		return ""
	}
	if recordedAt.Sub(current.StartedAt.Time) >= time.Duration(current.LimitDurationS)*time.Second {
		return "duration_limit"
	}
	return ""
}

func (e *Engine) startRecorded(ctx context.Context, request StartRequest, claimToken uuid.UUID) (Response, bool, error) {
	sessionExists := false
	if existing, err := e.repo.GetSession(ctx, uuidType(request.SessionRef)); err == nil {
		if sameSession(existing, request) {
			sessionExists = true
			if existing.State == StateActive || existing.State == StateStopping || existing.State == StateStopped {
				return commandResponse(http.StatusAccepted, existing.State, "accepted"), true, nil
			}
		} else {
			return commandResponse(http.StatusConflict, StateFailed, "session_ref_reused"), true, nil
		}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return Response{}, false, err
	}

	charger, ok := e.chargers.Get(request.ChargerID)
	if !ok || charger.ConnectionState != registry.StateConnected {
		if sessionExists {
			if err := e.failPendingStart(ctx, request, "charger_offline"); err != nil {
				return Response{}, false, err
			}
		}
		return commandResponse(http.StatusConflict, StateFailed, "charger_offline"), true, nil
	}
	connector, ok := charger.Connectors[int(request.ConnectorID)]
	if !ok || connector.Status != "Available" {
		if sessionExists {
			if err := e.failPendingStart(ctx, request, "connector_busy"); err != nil {
				return Response{}, false, err
			}
		}
		return commandResponse(http.StatusConflict, StateFailed, "connector_busy"), true, nil
	}

	if !sessionExists {
		_, err := e.repo.CreateSession(ctx, store.CreateSessionParams{
			SessionRef:     uuidType(request.SessionRef),
			StartCommandID: uuidType(request.CommandID),
			ChargerID:      request.ChargerID,
			ConnectorID:    request.ConnectorID,
			IDTag:          request.IDTag,
			LimitEnergyWh:  request.Limits.MaxEnergyWh,
			LimitDurationS: request.Limits.MaxDurationS,
		})
		if err != nil {
			if isUniqueViolation(err) {
				return commandResponse(http.StatusConflict, StateFailed, "connector_busy"), true, nil
			}
			return Response{}, false, err
		}
	}

	accepted, err := e.commands.RemoteStart(
		ctx,
		request.ChargerID,
		int(request.ConnectorID),
		request.IDTag,
		func() error { return e.markCommandDispatching(ctx, request.CommandID, claimToken) },
	)
	if errors.Is(err, errDispatchNotStarted) {
		return commandResponse(http.StatusAccepted, StateStartRequested, "processing"), false, nil
	}
	resultCtx, cancel := commandContext(ctx)
	defer cancel()
	current, getErr := e.repo.GetSession(resultCtx, uuidType(request.SessionRef))
	if getErr != nil {
		return Response{}, false, getErr
	}
	if current.State == StateActive || current.State == StateStopping || current.State == StateStopped {
		return commandResponse(http.StatusAccepted, StateStartRequested, "accepted"), true, nil
	}
	if isAmbiguous(err) {
		return commandResponse(http.StatusAccepted, StateStartRequested, "processing"), false, nil
	}
	if err != nil || !accepted {
		reason := commandFailure(err)
		changed, failErr := e.repo.FailSession(resultCtx, store.FailSessionParams{
			SessionRef: uuidType(request.SessionRef),
			StopReason: pgtype.Text{String: reason, Valid: true},
		})
		if failErr != nil {
			return Response{}, false, failErr
		}
		if changed == 0 {
			return e.reconcileStartResult(resultCtx, request)
		}
		return commandResponse(http.StatusAccepted, StateFailed, reason), true, nil
	}
	return commandResponse(http.StatusAccepted, StateStartRequested, "accepted"), true, nil
}

func (e *Engine) stopRecorded(ctx context.Context, request StopRequest, claimToken uuid.UUID) (Response, bool, error) {
	current, err := e.repo.GetSession(ctx, uuidType(request.SessionRef))
	if errors.Is(err, pgx.ErrNoRows) {
		return commandResponse(http.StatusNotFound, "not_found", ""), true, nil
	}
	if err != nil {
		return Response{}, false, err
	}
	if current.State == StateStopped {
		return commandResponse(http.StatusAccepted, StateStopped, "accepted"), true, nil
	}
	if !current.OcppTransactionID.Valid || (current.State != StateActive && current.State != StateStopping) {
		return commandResponse(http.StatusConflict, current.State, "session_not_active"), true, nil
	}
	if current.State == StateStopping && !sameUUID(current.StopCommandID, request.CommandID) {
		return commandResponse(http.StatusAccepted, StateStopping, "processing"), false, nil
	}
	if current.State == StateActive {
		current, err = e.repo.MarkSessionStopping(ctx, store.MarkSessionStoppingParams{
			SessionRef:    uuidType(request.SessionRef),
			StopSource:    pgtype.Text{String: "remote", Valid: true},
			StopCommandID: uuidType(request.CommandID),
		})
		if err != nil {
			if !errors.Is(err, pgx.ErrNoRows) {
				return Response{}, false, err
			}
			current, err = e.repo.GetSession(ctx, uuidType(request.SessionRef))
			if err != nil {
				return Response{}, false, err
			}
			if current.State == StateStopped {
				return commandResponse(http.StatusAccepted, StateStopped, "accepted"), true, nil
			}
			if current.State != StateStopping || !sameUUID(current.StopCommandID, request.CommandID) {
				return commandResponse(http.StatusAccepted, current.State, "processing"), false, nil
			}
		}
	}
	e.cancelDuration(current.SessionRef)

	accepted, err := e.commands.RemoteStop(
		ctx,
		current.ChargerID,
		int(current.OcppTransactionID.Int32),
		func() error { return e.markCommandDispatching(ctx, request.CommandID, claimToken) },
	)
	if errors.Is(err, errDispatchNotStarted) {
		return commandResponse(http.StatusAccepted, StateStopping, "processing"), false, nil
	}
	resultCtx, cancel := commandContext(ctx)
	defer cancel()
	current, getErr := e.repo.GetSession(resultCtx, uuidType(request.SessionRef))
	if getErr != nil {
		return Response{}, false, getErr
	}
	if current.State == StateStopped {
		return commandResponse(http.StatusAccepted, StateStopping, "accepted"), true, nil
	}
	if isAmbiguous(err) {
		return commandResponse(http.StatusAccepted, StateStopping, "processing"), false, nil
	}
	if err != nil || !accepted {
		changed, restoreErr := e.repo.RestoreSessionActive(resultCtx, uuidType(request.SessionRef))
		if restoreErr != nil {
			return Response{}, false, restoreErr
		}
		if changed == 0 {
			return e.reconcileStopResult(resultCtx, request)
		}
		e.scheduleDuration(current)
		return commandResponse(http.StatusAccepted, StateActive, commandFailure(err)), true, nil
	}
	return commandResponse(http.StatusAccepted, StateStopping, "accepted"), true, nil
}

func (e *Engine) recordCommand(ctx context.Context, id uuid.UUID, commandType string, raw []byte) (bool, bool, uuid.UUID, Response, error) {
	_, err := e.repo.InsertCommand(ctx, store.InsertCommandParams{
		CommandID:   uuidType(id),
		CommandType: commandType,
		Request:     raw,
	})
	if err != nil {
		return false, false, uuid.Nil, Response{}, err
	}

	existing, err := e.repo.GetCommand(ctx, uuidType(id))
	if err != nil {
		return false, false, uuid.Nil, Response{}, err
	}
	if existing.CommandType != commandType || !sameJSON(existing.Request, raw) {
		return true, false, uuid.Nil, commandResponse(http.StatusConflict, "conflict", "command_id_reused"), nil
	}
	if existing.HttpStatus.Valid && len(existing.Result) > 0 {
		var result Result
		if err := json.Unmarshal(existing.Result, &result); err != nil {
			return false, false, uuid.Nil, Response{}, err
		}
		return true, false, uuid.Nil, Response{Status: int(existing.HttpStatus.Int32), Result: result}, nil
	}
	if existing.State == "dispatching" {
		return false, true, uuid.UUID(existing.ClaimToken.Bytes), Response{}, nil
	}
	claimToken := uuid.New()
	if _, err := e.repo.ClaimCommand(ctx, store.ClaimCommandParams{
		CommandID:  uuidType(id),
		ClaimToken: uuidType(claimToken),
	}); errors.Is(err, pgx.ErrNoRows) {
		return true, false, uuid.Nil, commandResponse(http.StatusAccepted, "processing", ""), nil
	} else if err != nil {
		return false, false, uuid.Nil, Response{}, err
	}
	return false, false, claimToken, Response{}, nil
}

func (e *Engine) finalizeCommand(ctx context.Context, id, claimToken uuid.UUID, response Response, complete bool) (Response, error) {
	if !complete {
		return response, nil
	}
	raw, err := json.Marshal(response.Result)
	if err != nil {
		return Response{}, err
	}
	changed, err := e.repo.CompleteCommand(ctx, store.CompleteCommandParams{
		CommandID:  uuidType(id),
		Result:     raw,
		HttpStatus: pgtype.Int4{Int32: int32(response.Status), Valid: true},
		ClaimToken: uuidType(claimToken),
	})
	if err != nil {
		return Response{}, err
	}
	if changed == 1 {
		return response, nil
	}

	command, err := e.repo.GetCommand(ctx, uuidType(id))
	if err != nil {
		return Response{}, err
	}
	if command.State != "completed" {
		return commandResponse(http.StatusAccepted, "processing", ""), nil
	}
	var persisted Result
	if err := json.Unmarshal(command.Result, &persisted); err != nil {
		return Response{}, err
	}
	return Response{Status: int(command.HttpStatus.Int32), Result: persisted}, nil
}

func (e *Engine) markCommandDispatching(ctx context.Context, id, claimToken uuid.UUID) error {
	transitionCtx, cancel := commandContext(ctx)
	defer cancel()
	params := store.MarkCommandDispatchingParams{
		CommandID:  uuidType(id),
		ClaimToken: uuidType(claimToken),
	}
	if _, err := e.repo.MarkCommandDispatching(transitionCtx, params); err == nil {
		return nil
	} else {
		reconcileCtx, reconcileCancel := commandContext(ctx)
		defer reconcileCancel()
		command, reconcileErr := e.repo.GetCommand(reconcileCtx, uuidType(id))
		if reconcileErr == nil && command.State == "dispatching" && sameUUID(command.ClaimToken, claimToken) {
			return nil
		}
		return errors.Join(errDispatchNotStarted, err, reconcileErr)
	}
}

func validateStart(request StartRequest) error {
	switch {
	case request.SessionRef == uuid.Nil:
		return errors.New("session_ref is required")
	case request.ChargerID == "":
		return errors.New("charger_id is required")
	case request.ConnectorID <= 0:
		return errors.New("connector_id must be positive")
	case request.IDTag == "" || len(request.IDTag) > 20:
		return errors.New("id_tag must contain 1 to 20 characters")
	case request.Limits.MaxEnergyWh <= 0:
		return errors.New("max_energy_wh must be positive")
	case request.Limits.MaxDurationS <= 0:
		return errors.New("max_duration_s must be positive")
	default:
		return nil
	}
}

func invalidResponse(err error) Response {
	return Response{
		Status: http.StatusUnprocessableEntity,
		Result: Result{State: "invalid", Detail: err.Error()},
	}
}

func commandResponse(status int, state, result string) Response {
	return Response{
		Status: status,
		Result: Result{State: state, Result: result},
	}
}

func commandFailure(err error) string {
	if err != nil {
		return "charger_offline"
	}
	return "rejected"
}

func isAmbiguous(err error) bool {
	return errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled)
}

func commandContext(requestContext context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(requestContext), commandWorkTimeout)
}

func sameSession(session store.Session, request StartRequest) bool {
	return sameUUID(session.StartCommandID, request.CommandID) &&
		session.ChargerID == request.ChargerID &&
		session.ConnectorID == request.ConnectorID &&
		session.IDTag == request.IDTag &&
		session.LimitEnergyWh == request.Limits.MaxEnergyWh &&
		session.LimitDurationS == request.Limits.MaxDurationS
}

func (e *Engine) failPendingStart(ctx context.Context, request StartRequest, reason string) error {
	current, err := e.repo.GetSession(ctx, uuidType(request.SessionRef))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if !sameSession(current, request) || current.State != StateStartRequested {
		return nil
	}
	_, err = e.repo.FailSession(ctx, store.FailSessionParams{
		SessionRef: current.SessionRef,
		StopReason: pgtype.Text{String: reason, Valid: true},
	})
	return err
}

func uuidType(value uuid.UUID) pgtype.UUID {
	return pgtype.UUID{Bytes: value, Valid: value != uuid.Nil}
}

func sameUUID(value pgtype.UUID, expected uuid.UUID) bool {
	return value.Valid && uuid.UUID(value.Bytes) == expected
}

func textValue(value pgtype.Text, fallback string) string {
	if value.Valid {
		return value.String
	}
	return fallback
}

func sameJSON(a, b []byte) bool {
	var left, right any
	if json.Unmarshal(a, &left) != nil || json.Unmarshal(b, &right) != nil {
		return bytes.Equal(a, b)
	}
	return reflect.DeepEqual(left, right)
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
