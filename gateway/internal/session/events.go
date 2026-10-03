package session

import (
	"context"
	"encoding/json"
	"time"

	"github.com/arnabry11/ev-charging/gateway/internal/store"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

const (
	EventSessionStarted = "session.started"
	EventMeterValues    = "session.meter_values"
	EventSessionStopped = "session.stopped"
	EventCommandResult  = "command.result"
)

type limitsPayload struct {
	MaxEnergyWh  int64 `json:"max_energy_wh"`
	MaxDurationS int32 `json:"max_duration_s"`
}

func (e *Engine) within(ctx context.Context, fn func(repository) error) error {
	if e.work == nil {
		return fn(e.repo)
	}
	return e.work.Within(ctx, fn)
}

func (e *Engine) WithUnitOfWork(work unitOfWork) *Engine {
	e.work = work
	return e
}

func appendOutboxEvent(ctx context.Context, repo repository, sessionRef pgtype.UUID, eventType string, payload any, occurredAt time.Time) error {
	if !sessionRef.Valid || uuid.UUID(sessionRef.Bytes) == uuid.Nil {
		return nil
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = repo.AppendOutboxEvent(ctx, store.AppendOutboxEventParams{
		SessionRef: sessionRef,
		EventType:  eventType,
		Payload:    raw,
		OccurredAt: pgtype.Timestamptz{Time: occurredAt.UTC(), Valid: true},
	})
	return err
}

func sessionStartedPayload(current store.Session) map[string]any {
	return map[string]any{
		"charger_id":          current.ChargerID,
		"connector_id":        current.ConnectorID,
		"id_tag":              current.IDTag,
		"ocpp_transaction_id": current.OcppTransactionID.Int32,
		"meter_start_wh":      current.MeterStartWh.Int64,
		"started_at":          current.StartedAt.Time.UTC().Format(time.RFC3339),
		"limits": limitsPayload{
			MaxEnergyWh:  current.LimitEnergyWh,
			MaxDurationS: current.LimitDurationS,
		},
	}
}

func meterValuesPayload(current store.Session, recordedAt time.Time) map[string]any {
	return map[string]any{
		"charger_id":          current.ChargerID,
		"connector_id":        current.ConnectorID,
		"ocpp_transaction_id": current.OcppTransactionID.Int32,
		"meter_start_wh":      current.MeterStartWh.Int64,
		"energy_wh":           current.LastEnergyWh.Int64,
		"recorded_at":         recordedAt.UTC().Format(time.RFC3339),
	}
}

func sessionStoppedPayload(current store.Session) map[string]any {
	return map[string]any{
		"charger_id":          current.ChargerID,
		"connector_id":        current.ConnectorID,
		"ocpp_transaction_id": current.OcppTransactionID.Int32,
		"meter_start_wh":      current.MeterStartWh.Int64,
		"meter_stop_wh":       current.MeterStopWh.Int64,
		"started_at":          current.StartedAt.Time.UTC().Format(time.RFC3339),
		"stopped_at":          current.StoppedAt.Time.UTC().Format(time.RFC3339),
		"stop_reason":         current.StopReason.String,
		"stop_source":         current.StopSource.String,
	}
}

func commandResultPayload(commandID uuid.UUID, commandType string, response Response) map[string]any {
	return map[string]any{
		"command_id":   commandID.String(),
		"command_type": commandType,
		"http_status":  response.Status,
		"state":        response.Result.State,
		"result":       response.Result.Result,
		"detail":       response.Result.Detail,
	}
}
