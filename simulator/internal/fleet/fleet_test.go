package fleet

import (
	"reflect"
	"testing"
)

func TestChargerIDsCountUpFromTheBaseID(t *testing.T) {
	t.Parallel()

	got, err := ChargerIDs("CHG-MUM-0001", 3)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"CHG-MUM-0001", "CHG-MUM-0002", "CHG-MUM-0003"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ids = %v, want %v", got, want)
	}
}

func TestChargerIDsKeepTheNumberWidth(t *testing.T) {
	t.Parallel()

	got, err := ChargerIDs("CHG-MUM-0009", 2)
	if err != nil {
		t.Fatal(err)
	}
	if got[1] != "CHG-MUM-0010" {
		t.Fatalf("second id = %s", got[1])
	}
}

func TestChargerIDsRejectABaseWithoutANumber(t *testing.T) {
	t.Parallel()

	if _, err := ChargerIDs("demo", 2); err == nil {
		t.Fatal("expected an error for a base id without a trailing number")
	}
	if got, err := ChargerIDs("demo", 1); err != nil || len(got) != 1 || got[0] != "demo" {
		t.Fatalf("a single charger keeps any id: %v, %v", got, err)
	}
}

func TestChargerIDsRejectANonPositiveCount(t *testing.T) {
	t.Parallel()

	if _, err := ChargerIDs("CHG-MUM-0001", 0); err == nil {
		t.Fatal("expected an error for a zero count")
	}
}

func TestPowerWSpreadsChargersAroundTheBase(t *testing.T) {
	t.Parallel()

	if got := PowerW(7_200, 0); got != 7_200 {
		t.Fatalf("first charger keeps the base power, got %d", got)
	}
	seen := map[int64]bool{}
	for i := 0; i < 5; i++ {
		seen[PowerW(7_200, i)] = true
	}
	if len(seen) != 5 {
		t.Fatalf("expected five distinct power levels, got %v", seen)
	}
	if PowerW(7_200, 5) != PowerW(7_200, 0) {
		t.Fatal("the profile repeats every five chargers")
	}
}
