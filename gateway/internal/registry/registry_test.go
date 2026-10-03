package registry

import (
	"testing"
	"time"
)

func TestConnectedBootHeartbeatDisconnect(t *testing.T) {
	t.Parallel()

	reg := New()
	reg.Connected("CHG-1")
	reg.Booted("CHG-1", "Vendor", "Model", "1.0", 10)
	reg.Heartbeat("CHG-1")

	got, ok := reg.Get("CHG-1")
	if !ok {
		t.Fatal("expected charger")
	}
	if got.ConnectionState != StateConnected {
		t.Fatalf("state = %s", got.ConnectionState)
	}
	if got.Vendor != "Vendor" || got.Model != "Model" {
		t.Fatalf("boot fields = %+v", got)
	}

	reg.Disconnected("CHG-1")
	got, _ = reg.Get("CHG-1")
	if got.ConnectionState != StateDisconnected {
		t.Fatalf("state = %s", got.ConnectionState)
	}
}

func TestConnectorStatus(t *testing.T) {
	t.Parallel()

	reg := New()
	reg.Connected("CHG-1")
	reportedAt := time.Date(2026, time.October, 3, 7, 30, 0, 0, time.UTC)
	reg.ConnectorStatus("CHG-1", Connector{
		ID:        1,
		Status:    "Available",
		ErrorCode: "NoError",
		VendorID:  "POCSim",
	}, reportedAt)

	got, ok := reg.Get("CHG-1")
	if !ok {
		t.Fatal("expected charger")
	}
	connector, ok := got.Connectors[1]
	if !ok {
		t.Fatal("expected connector")
	}
	if connector.Status != "Available" || connector.ErrorCode != "NoError" {
		t.Fatalf("connector = %+v", connector)
	}
	if !connector.UpdatedAt.Equal(reportedAt) {
		t.Fatalf("updated at = %v, want %v", connector.UpdatedAt, reportedAt)
	}
	if connector.VendorID != "POCSim" {
		t.Fatalf("vendor id = %q", connector.VendorID)
	}

	delete(got.Connectors, 1)
	fresh, _ := reg.Get("CHG-1")
	if _, ok := fresh.Connectors[1]; !ok {
		t.Fatal("Get leaked the registry's connector map")
	}
}
