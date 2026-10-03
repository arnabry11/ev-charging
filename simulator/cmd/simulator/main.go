package main

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/arnabry11/ev-charging/simulator/internal/charger"
	"github.com/lorenzodonini/ocpp-go/ocpp1.6"
	"github.com/lorenzodonini/ocpp-go/ocpp1.6/core"
	"github.com/lorenzodonini/ocpp-go/ws"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	chargerID := getenv("CHARGER_ID", "CHG-MUM-0001")
	password := getenv("CHARGER_PASSWORD", "demo-password")
	gatewayURL := getenv("GATEWAY_URL", "ws://gateway:9000")
	httpAddr := getenv("HTTP_ADDR", ":8081")

	client := ws.NewClient()
	client.SetBasicAuth(chargerID, password)
	cp := ocpp16.NewChargePoint(chargerID, nil, client)
	controller := charger.New(cp, charger.Config{
		PowerW:                  getenvInt64("SIM_POWER_W", 7_200),
		MeterStartWh:            getenvInt64("SIM_METER_START_WH", 100_000),
		TickInterval:            time.Duration(getenvInt64("SIM_TICK_INTERVAL_MS", 1_000)) * time.Millisecond,
		SimulatedSecondsPerTick: getenvInt64("SIM_SECONDS_PER_TICK", 60),
		Logger:                  logger,
	})
	cp.SetCoreHandler(coreHandler{controller: controller})

	ready := make(chan struct{})
	go func() {
		mux := http.NewServeMux()
		mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
			select {
			case <-ready:
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok", "charger_id": chargerID})
			default:
				http.Error(w, `{"status":"starting"}`, http.StatusServiceUnavailable)
			}
		})
		if err := http.ListenAndServe(httpAddr, mux); err != nil {
			logger.Error("simulator http stopped", "err", err)
			os.Exit(1)
		}
	}()

	if err := cp.Start(gatewayURL); err != nil {
		logger.Error("connect to gateway", "err", err, "url", gatewayURL)
		os.Exit(1)
	}
	defer cp.Stop()

	boot, err := cp.BootNotification("POC-AC", "ev-charging-sim")
	if err != nil {
		logger.Error("BootNotification", "err", err)
		os.Exit(1)
	}
	logger.Info("booted", "status", boot.Status, "heartbeat_s", boot.Interval)

	if _, err := cp.StatusNotification(1, core.NoError, core.ChargePointStatusAvailable); err != nil {
		logger.Error("StatusNotification", "err", err)
		os.Exit(1)
	}
	logger.Info("connector available", "charger_id", chargerID, "connector_id", 1)
	close(ready)

	interval := time.Duration(boot.Interval) * time.Second
	if interval <= 0 {
		interval = 10 * time.Second
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-stop:
			logger.Info("simulator stopping")
			return
		case <-ticker.C:
			if _, err := cp.Heartbeat(); err != nil {
				logger.Error("Heartbeat", "err", err)
			} else {
				logger.Info("heartbeat sent", "charger_id", chargerID)
			}
		}
	}
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getenvInt64(key string, fallback int64) int64 {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return fallback
	}
	return value
}

type coreHandler struct {
	controller *charger.Controller
}

func (coreHandler) OnChangeAvailability(*core.ChangeAvailabilityRequest) (*core.ChangeAvailabilityConfirmation, error) {
	return core.NewChangeAvailabilityConfirmation(core.AvailabilityStatusRejected), nil
}
func (coreHandler) OnChangeConfiguration(*core.ChangeConfigurationRequest) (*core.ChangeConfigurationConfirmation, error) {
	return core.NewChangeConfigurationConfirmation(core.ConfigurationStatusRejected), nil
}
func (coreHandler) OnClearCache(*core.ClearCacheRequest) (*core.ClearCacheConfirmation, error) {
	return core.NewClearCacheConfirmation(core.ClearCacheStatusRejected), nil
}
func (coreHandler) OnDataTransfer(*core.DataTransferRequest) (*core.DataTransferConfirmation, error) {
	return core.NewDataTransferConfirmation(core.DataTransferStatusRejected), nil
}
func (coreHandler) OnGetConfiguration(*core.GetConfigurationRequest) (*core.GetConfigurationConfirmation, error) {
	return core.NewGetConfigurationConfirmation(nil), nil
}
func (h coreHandler) OnRemoteStartTransaction(request *core.RemoteStartTransactionRequest) (*core.RemoteStartTransactionConfirmation, error) {
	return core.NewRemoteStartTransactionConfirmation(h.controller.RemoteStart(request)), nil
}
func (h coreHandler) OnRemoteStopTransaction(request *core.RemoteStopTransactionRequest) (*core.RemoteStopTransactionConfirmation, error) {
	return core.NewRemoteStopTransactionConfirmation(h.controller.RemoteStop(request)), nil
}
func (coreHandler) OnReset(*core.ResetRequest) (*core.ResetConfirmation, error) {
	return core.NewResetConfirmation(core.ResetStatusRejected), nil
}
func (coreHandler) OnUnlockConnector(*core.UnlockConnectorRequest) (*core.UnlockConnectorConfirmation, error) {
	return core.NewUnlockConnectorConfirmation(core.UnlockStatusNotSupported), nil
}
