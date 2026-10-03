package auth

import "testing"

func TestParseAndValid(t *testing.T) {
	t.Parallel()

	store, err := Parse("CHG-MUM-0001:demo-password")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if !store.Valid("CHG-MUM-0001", "demo-password") {
		t.Fatal("expected matching credentials to be valid")
	}
	if store.Valid("CHG-MUM-0001", "wrong") {
		t.Fatal("expected wrong password to be rejected")
	}
	if store.Valid("unknown", "demo-password") {
		t.Fatal("expected unknown charger to be rejected")
	}
}

func TestParseRejectsEmpty(t *testing.T) {
	t.Parallel()

	if _, err := Parse("  "); err == nil {
		t.Fatal("expected error")
	}
	if _, err := Parse("no-colon"); err == nil {
		t.Fatal("expected error")
	}
}
