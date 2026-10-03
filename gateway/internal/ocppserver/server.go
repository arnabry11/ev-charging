package ocppserver

import (
	"log/slog"
	"net/http"

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
	Logger             *slog.Logger
}

type Server struct {
	cs     ocpp16.CentralSystem
	cfg    Config
	logger *slog.Logger
}

func New(cfg Config) *Server {
	if cfg.Path == "" {
		cfg.Path = DefaultPath
	}
	if cfg.HeartbeatIntervalS <= 0 {
		cfg.HeartbeatIntervalS = 10
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

	return &Server{cs: cs, cfg: cfg, logger: logger}
}

func (s *Server) Start() {
	s.logger.Info("ocpp listening", "port", s.cfg.Port, "path", s.cfg.Path)
	s.cs.Start(s.cfg.Port, s.cfg.Path)
}

type coreHandler struct {
	registry           *registry.Registry
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

func (h *coreHandler) OnMeterValues(_ string, _ *core.MeterValuesRequest) (*core.MeterValuesConfirmation, error) {
	return core.NewMeterValuesConfirmation(), nil
}

func (h *coreHandler) OnStatusNotification(chargerID string, _ *core.StatusNotificationRequest) (*core.StatusNotificationConfirmation, error) {
	h.registry.Heartbeat(chargerID)
	return core.NewStatusNotificationConfirmation(), nil
}

func (h *coreHandler) OnStartTransaction(_ string, _ *core.StartTransactionRequest) (*core.StartTransactionConfirmation, error) {
	return core.NewStartTransactionConfirmation(types.NewIdTagInfo(types.AuthorizationStatusInvalid), 0), nil
}

func (h *coreHandler) OnStopTransaction(_ string, _ *core.StopTransactionRequest) (*core.StopTransactionConfirmation, error) {
	return core.NewStopTransactionConfirmation(), nil
}
