package ocppserver

import (
	"context"
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/arnabry11/ev-charging/gateway/internal/auth"
	"github.com/arnabry11/ev-charging/gateway/internal/registry"
	ocpp16 "github.com/lorenzodonini/ocpp-go/ocpp1.6"
	"github.com/lorenzodonini/ocpp-go/ocpp1.6/core"
	"github.com/lorenzodonini/ocpp-go/ocpp1.6/types"
	"github.com/lorenzodonini/ocpp-go/ws"
)

const DefaultPath = "/{ws}"

type Config struct {
	Port               int
	Path               string
	HeartbeatIntervalS int
	Auth               *auth.Store
	Registry           *registry.Registry
	Sessions           sessionHandler
	Logger             *slog.Logger
	CommandTimeout     time.Duration
}

type Server struct {
	cs         ocpp16.CentralSystem
	cfg        Config
	handler    *coreHandler
	logger     *slog.Logger
	dispatcher *commandDispatcher
}

type sessionHandler interface {
	Activate(context.Context, string, int, string, int64, time.Time) (int, error)
	Meter(context.Context, string, int, int64, time.Time) error
	Finish(context.Context, string, int, int64, time.Time, string) error
}

func New(cfg Config) *Server {
	if cfg.Path == "" {
		cfg.Path = DefaultPath
	}
	if cfg.HeartbeatIntervalS <= 0 {
		cfg.HeartbeatIntervalS = 10
	}
	if cfg.CommandTimeout <= 0 {
		cfg.CommandTimeout = 10 * time.Second
	}
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}

	wsServer := ws.NewServer()
	wsServer.SetCheckOriginHandler(func(_ *http.Request) bool { return true })
	wsServer.SetBasicAuthHandler(func(username, password string) bool {
		return cfg.Auth.Valid(username, password)
	})
	wsServer.SetCheckClientHandler(func(id string, r *http.Request) bool {
		username, _, ok := r.BasicAuth()
		if !ok || username != id {
			return false
		}
		return cfg.Auth.Known(id)
	})

	cs := ocpp16.NewCentralSystem(nil, wsServer)
	handler := &coreHandler{
		registry:           cfg.Registry,
		sessions:           cfg.Sessions,
		heartbeatIntervalS: cfg.HeartbeatIntervalS,
		logger:             logger,
	}
	cs.SetCoreHandler(handler)
	cs.SetNewChargePointHandler(func(cp ocpp16.ChargePointConnection) {
		logger.Info("charger connected", "charger_id", cp.ID())
		cfg.Registry.Connected(cp.ID())
	})
	cs.SetChargePointDisconnectedHandler(func(cp ocpp16.ChargePointConnection) {
		logger.Info("charger disconnected", "charger_id", cp.ID())
		cfg.Registry.Disconnected(cp.ID())
	})

	return &Server{
		cs:         cs,
		cfg:        cfg,
		handler:    handler,
		logger:     logger,
		dispatcher: newCommandDispatcher(),
	}
}

func (s *Server) SetSessionHandler(handler sessionHandler) {
	s.handler.sessions = handler
}

func (s *Server) Start() {
	s.logger.Info("ocpp listening", "port", s.cfg.Port, "path", s.cfg.Path)
	s.cs.Start(s.cfg.Port, s.cfg.Path)
}

func (s *Server) RemoteStart(ctx context.Context, chargerID string, connectorID int, idTag string, beforeSend func() error) (bool, error) {
	return s.dispatcher.Do(ctx, chargerID, func() (bool, error) {
		if err := beforeSend(); err != nil {
			return false, err
		}
		return s.remoteStart(context.Background(), chargerID, connectorID, idTag)
	})
}

func (s *Server) remoteStart(ctx context.Context, chargerID string, connectorID int, idTag string) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, s.cfg.CommandTimeout)
	defer cancel()

	result := make(chan commandResult, 1)
	err := s.cs.RemoteStartTransaction(
		chargerID,
		func(confirmation *core.RemoteStartTransactionConfirmation, err error) {
			if err != nil {
				result <- commandResult{err: err}
				return
			}
			result <- commandResult{accepted: confirmation.Status == types.RemoteStartStopStatusAccepted}
		},
		idTag,
		func(request *core.RemoteStartTransactionRequest) {
			request.ConnectorId = &connectorID
		},
	)
	if err != nil {
		return false, err
	}
	return awaitCommand(ctx, result)
}

func (s *Server) RemoteStop(ctx context.Context, chargerID string, transactionID int, beforeSend func() error) (bool, error) {
	return s.dispatcher.Do(ctx, chargerID, func() (bool, error) {
		if err := beforeSend(); err != nil {
			return false, err
		}
		return s.remoteStop(context.Background(), chargerID, transactionID)
	})
}

func (s *Server) remoteStop(ctx context.Context, chargerID string, transactionID int) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, s.cfg.CommandTimeout)
	defer cancel()

	result := make(chan commandResult, 1)
	err := s.cs.RemoteStopTransaction(
		chargerID,
		func(confirmation *core.RemoteStopTransactionConfirmation, err error) {
			if err != nil {
				result <- commandResult{err: err}
				return
			}
			result <- commandResult{accepted: confirmation.Status == types.RemoteStartStopStatusAccepted}
		},
		transactionID,
	)
	if err != nil {
		return false, err
	}
	return awaitCommand(ctx, result)
}

type commandResult struct {
	accepted bool
	err      error
}

func awaitCommand(ctx context.Context, result <-chan commandResult) (bool, error) {
	select {
	case <-ctx.Done():
		return false, ctx.Err()
	case response := <-result:
		return response.accepted, response.err
	}
}

func energyReading(request *core.MeterValuesRequest) (int64, time.Time, bool) {
	var energyWh int64
	var recordedAt time.Time
	found := false
	for _, meterValue := range request.MeterValue {
		for _, sample := range meterValue.SampledValue {
			value, ok := sampledEnergyWh(sample)
			if !ok {
				continue
			}
			timestamp := meterValue.Timestamp.Time
			if !found || !timestamp.Before(recordedAt) {
				energyWh = value
				recordedAt = timestamp
				found = true
			}
		}
	}
	return energyWh, recordedAt, found
}

func sampledEnergyWh(sample types.SampledValue) (int64, bool) {
	if sample.Measurand != "" && sample.Measurand != types.MeasurandEnergyActiveImportRegister {
		return 0, false
	}

	var multiplier float64
	switch sample.Unit {
	case "", types.UnitOfMeasureWh:
		multiplier = 1
	case types.UnitOfMeasureKWh:
		multiplier = 1_000
	default:
		return 0, false
	}

	value, err := strconv.ParseFloat(sample.Value, 64)
	if err != nil || value < 0 || math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, false
	}
	return int64(math.Round(value * multiplier)), true
}

type coreHandler struct {
	registry           *registry.Registry
	sessions           sessionHandler
	heartbeatIntervalS int
	logger             *slog.Logger
}

func (h *coreHandler) OnBootNotification(chargerID string, request *core.BootNotificationRequest) (*core.BootNotificationConfirmation, error) {
	h.logger.Info("BootNotification", "charger_id", chargerID, "vendor", request.ChargePointVendor, "model", request.ChargePointModel)
	h.registry.Booted(chargerID, request.ChargePointVendor, request.ChargePointModel, request.FirmwareVersion, h.heartbeatIntervalS)
	return core.NewBootNotificationConfirmation(types.NewDateTime(nowUTC()), h.heartbeatIntervalS, core.RegistrationStatusAccepted), nil
}

func (h *coreHandler) OnHeartbeat(chargerID string, _ *core.HeartbeatRequest) (*core.HeartbeatConfirmation, error) {
	h.logger.Info("Heartbeat", "charger_id", chargerID)
	h.registry.Heartbeat(chargerID)
	return core.NewHeartbeatConfirmation(types.NewDateTime(nowUTC())), nil
}

func (h *coreHandler) OnAuthorize(_ string, _ *core.AuthorizeRequest) (*core.AuthorizeConfirmation, error) {
	return core.NewAuthorizationConfirmation(types.NewIdTagInfo(types.AuthorizationStatusInvalid)), nil
}

func (h *coreHandler) OnDataTransfer(_ string, _ *core.DataTransferRequest) (*core.DataTransferConfirmation, error) {
	return core.NewDataTransferConfirmation(core.DataTransferStatusRejected), nil
}

func (h *coreHandler) OnMeterValues(chargerID string, request *core.MeterValuesRequest) (*core.MeterValuesConfirmation, error) {
	if h.sessions == nil || request.TransactionId == nil {
		return core.NewMeterValuesConfirmation(), nil
	}
	energyWh, recordedAt, ok := energyReading(request)
	if !ok {
		return core.NewMeterValuesConfirmation(), nil
	}
	if err := h.sessions.Meter(
		context.Background(),
		chargerID,
		*request.TransactionId,
		energyWh,
		recordedAt,
	); err != nil {
		return nil, err
	}
	return core.NewMeterValuesConfirmation(), nil
}

func (h *coreHandler) OnStatusNotification(chargerID string, request *core.StatusNotificationRequest) (*core.StatusNotificationConfirmation, error) {
	h.logger.Info(
		"StatusNotification",
		"charger_id", chargerID,
		"connector_id", request.ConnectorId,
		"status", request.Status,
		"error_code", request.ErrorCode,
	)
	var reportedAt time.Time
	if request.Timestamp != nil {
		reportedAt = request.Timestamp.Time
	}
	h.registry.ConnectorStatus(chargerID, registry.Connector{
		ID:              request.ConnectorId,
		Status:          string(request.Status),
		ErrorCode:       string(request.ErrorCode),
		Info:            request.Info,
		VendorID:        request.VendorId,
		VendorErrorCode: request.VendorErrorCode,
	}, reportedAt)
	return core.NewStatusNotificationConfirmation(), nil
}

func (h *coreHandler) OnStartTransaction(chargerID string, request *core.StartTransactionRequest) (*core.StartTransactionConfirmation, error) {
	if h.sessions == nil {
		return core.NewStartTransactionConfirmation(types.NewIdTagInfo(types.AuthorizationStatusInvalid), 0), nil
	}
	transactionID, err := h.sessions.Activate(
		context.Background(),
		chargerID,
		request.ConnectorId,
		request.IdTag,
		int64(request.MeterStart),
		request.Timestamp.Time,
	)
	if err != nil {
		return nil, err
	}
	return core.NewStartTransactionConfirmation(
		types.NewIdTagInfo(types.AuthorizationStatusAccepted),
		transactionID,
	), nil
}

func (h *coreHandler) OnStopTransaction(chargerID string, request *core.StopTransactionRequest) (*core.StopTransactionConfirmation, error) {
	if h.sessions == nil {
		return core.NewStopTransactionConfirmation(), nil
	}
	if err := h.sessions.Finish(
		context.Background(),
		chargerID,
		request.TransactionId,
		int64(request.MeterStop),
		request.Timestamp.Time,
		string(request.Reason),
	); err != nil {
		return nil, err
	}
	return core.NewStopTransactionConfirmation(), nil
}
