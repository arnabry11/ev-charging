package registry

import "testing"

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
