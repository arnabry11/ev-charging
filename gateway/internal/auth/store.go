package auth

import (
	"crypto/subtle"
	"fmt"
	"strings"
)

// Store holds per-charger Basic auth passwords for Security Profile 1.
// Passwords are compared in constant time. Hashing lands with the gateway DB.
type Store struct {
	passwords map[string]string
}

func Parse(raw string) (*Store, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, fmt.Errorf("CHARGER_AUTH is empty")
	}

	passwords := make(map[string]string)
	for _, pair := range strings.Split(raw, ",") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		id, password, ok := strings.Cut(pair, ":")
		if !ok || id == "" || password == "" {
			return nil, fmt.Errorf("invalid CHARGER_AUTH entry %q, want charger_id:password", pair)
		}
		passwords[id] = password
	}
	if len(passwords) == 0 {
		return nil, fmt.Errorf("CHARGER_AUTH is empty")
	}
	return &Store{passwords: passwords}, nil
}

func (s *Store) Known(chargerID string) bool {
	_, ok := s.passwords[chargerID]
	return ok
}

func (s *Store) Valid(chargerID, password string) bool {
	want, ok := s.passwords[chargerID]
	if !ok {
		return false
	}
	if len(password) != len(want) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(password), []byte(want)) == 1
}
