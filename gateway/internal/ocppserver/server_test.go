package ocppserver_test

import (
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/arnabry11/ev-charging/gateway/internal/auth"
	"github.com/arnabry11/ev-charging/gateway/internal/ocppserver"
	"github.com/arnabry11/ev-charging/gateway/internal/registry"
	ocpp16 "github.com/lorenzodonini/ocpp-go/ocpp1.6"
	"github.com/lorenzodonini/ocpp-go/ocpp1.6/core"
	"github.com/lorenzodonini/ocpp-go/ocpp1.6/types"
	"github.com/lorenzodonini/ocpp-go/ws"
)

func TestBootAndHeartbeat(t *testing.T) {
	store, err := auth.Parse("CHG-TEST:secret")
	if err != nil {
		t.Fatal(err)
	}
	reg := registry.New()
	port := freePort(t)
	srv := ocppserver.New(ocppserver.Config{
		Port:               port,
		HeartbeatIntervalS: 10,
		Auth:               store,
		Registry:           reg,
	})
	go srv.Start()
	waitForPort(t, port)

	client := ws.NewClient()
	client.SetBasicAuth("CHG-TEST", "secret")
	cp := ocpp16.NewChargePoint("CHG-TEST", nil, client)
	cp.SetCoreHandler(stubChargePoint{})

	url := fmt.Sprintf("ws://127.0.0.1:%d", port)
	if err := cp.Start(url); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(cp.Stop)

	conf, err := cp.BootNotification("ev-charging", "POCSim")
	if err != nil {
		t.Fatalf("boot: %v", err)
	}
	if conf.Status != core.RegistrationStatusAccepted {
		t.Fatalf("boot status = %s", conf.Status)
	}
	if conf.Interval != 10 {
		t.Fatalf("interval = %d", conf.Interval)
	}

	if _, err := cp.Heartbeat(); err != nil {
		t.Fatalf("heartbeat: %v", err)
	}

	got, ok := waitCharger(t, reg, "CHG-TEST")
	if !ok {
		t.Fatal("charger not in registry")
	}
	if got.ConnectionState != registry.StateConnected {
		t.Fatalf("state = %s", got.ConnectionState)
	}
	if got.Vendor != "POCSim" || got.Model != "ev-charging" {
		t.Fatalf("boot fields = %+v", got)
	}
}

func TestRejectsWrongPassword(t *testing.T) {
	store, err := auth.Parse("CHG-TEST:secret")
	if err != nil {
		t.Fatal(err)
	}
	port := freePort(t)
	srv := ocppserver.New(ocppserver.Config{
		Port:     port,
		Auth:     store,
		Registry: registry.New(),
	})
	go srv.Start()
	waitForPort(t, port)

	client := ws.NewClient()
	client.SetBasicAuth("CHG-TEST", "wrong")
	cp := ocpp16.NewChargePoint("CHG-TEST", nil, client)
	cp.SetCoreHandler(stubChargePoint{})
	err = cp.Start(fmt.Sprintf("ws://127.0.0.1:%d", port))
	if err == nil {
		cp.Stop()
		t.Fatal("expected auth failure")
	}
}

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	return port
}

func waitForPort(t *testing.T, port int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 50*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("ocpp server did not listen on %s", addr)
}

func waitCharger(t *testing.T, reg *registry.Registry, id string) (registry.Charger, bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		got, ok := reg.Get(id)
		if ok && got.Vendor != "" {
			return got, true
		}
		time.Sleep(20 * time.Millisecond)
	}
	return reg.Get(id)
}

type stubChargePoint struct{}

func (stubChargePoint) OnChangeAvailability(*core.ChangeAvailabilityRequest) (*core.ChangeAvailabilityConfirmation, error) {
	return core.NewChangeAvailabilityConfirmation(core.AvailabilityStatusRejected), nil
}
func (stubChargePoint) OnChangeConfiguration(*core.ChangeConfigurationRequest) (*core.ChangeConfigurationConfirmation, error) {
	return core.NewChangeConfigurationConfirmation(core.ConfigurationStatusRejected), nil
}
func (stubChargePoint) OnClearCache(*core.ClearCacheRequest) (*core.ClearCacheConfirmation, error) {
	return core.NewClearCacheConfirmation(core.ClearCacheStatusRejected), nil
}
func (stubChargePoint) OnDataTransfer(*core.DataTransferRequest) (*core.DataTransferConfirmation, error) {
	return core.NewDataTransferConfirmation(core.DataTransferStatusRejected), nil
}
func (stubChargePoint) OnGetConfiguration(*core.GetConfigurationRequest) (*core.GetConfigurationConfirmation, error) {
	return core.NewGetConfigurationConfirmation(nil), nil
}
func (stubChargePoint) OnRemoteStartTransaction(*core.RemoteStartTransactionRequest) (*core.RemoteStartTransactionConfirmation, error) {
	return core.NewRemoteStartTransactionConfirmation(types.RemoteStartStopStatusRejected), nil
}
func (stubChargePoint) OnRemoteStopTransaction(*core.RemoteStopTransactionRequest) (*core.RemoteStopTransactionConfirmation, error) {
	return core.NewRemoteStopTransactionConfirmation(types.RemoteStartStopStatusRejected), nil
}
func (stubChargePoint) OnReset(*core.ResetRequest) (*core.ResetConfirmation, error) {
	return core.NewResetConfirmation(core.ResetStatusRejected), nil
}
func (stubChargePoint) OnUnlockConnector(*core.UnlockConnectorRequest) (*core.UnlockConnectorConfirmation, error) {
	return core.NewUnlockConnectorConfirmation(core.UnlockStatusNotSupported), nil
}
