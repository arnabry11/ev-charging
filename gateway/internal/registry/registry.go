package registry

import (
	"sync"
	"time"
)

const (
	StateConnected    = "connected"
	StateDisconnected = "disconnected"
)

type Charger struct {
	ID               string    `json:"charger_id"`
	ConnectionState  string    `json:"connection_state"`
	Vendor           string    `json:"vendor,omitempty"`
	Model            string    `json:"model,omitempty"`
	FirmwareVersion  string    `json:"firmware_version,omitempty"`
	LastSeenAt       time.Time `json:"last_seen_at"`
	LastBootAt       time.Time `json:"last_boot_at,omitempty"`
	HeartbeatInterval int      `json:"heartbeat_interval_s,omitempty"`
}

type Registry struct {
	mu       sync.RWMutex
	chargers map[string]Charger
}

func New() *Registry {
	return &Registry{chargers: make(map[string]Charger)}
}

func (r *Registry) Connected(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	c := r.chargers[id]
	c.ID = id
	c.ConnectionState = StateConnected
	c.LastSeenAt = time.Now().UTC()
	r.chargers[id] = c
}

func (r *Registry) Disconnected(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	c, ok := r.chargers[id]
	if !ok {
		return
	}
	c.ConnectionState = StateDisconnected
	c.LastSeenAt = time.Now().UTC()
	r.chargers[id] = c
}

func (r *Registry) Booted(id, vendor, model, firmware string, heartbeatInterval int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	c := r.chargers[id]
	c.ID = id
	c.ConnectionState = StateConnected
	c.Vendor = vendor
	c.Model = model
	c.FirmwareVersion = firmware
	c.HeartbeatInterval = heartbeatInterval
	now := time.Now().UTC()
	c.LastBootAt = now
	c.LastSeenAt = now
	r.chargers[id] = c
}

func (r *Registry) Heartbeat(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	c, ok := r.chargers[id]
	if !ok {
		c.ID = id
		c.ConnectionState = StateConnected
	}
	c.LastSeenAt = time.Now().UTC()
	r.chargers[id] = c
}

func (r *Registry) Get(id string) (Charger, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	c, ok := r.chargers[id]
	return c, ok
}
