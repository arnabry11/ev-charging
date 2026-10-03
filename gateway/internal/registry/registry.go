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
	ID                string
	ConnectionState   string
	Vendor            string
	Model             string
	FirmwareVersion   string
	LastSeenAt        time.Time
	LastBootAt        time.Time
	HeartbeatInterval int
	Connectors        map[int]Connector
}

type Connector struct {
	ID              int
	Status          string
	ErrorCode       string
	Info            string
	VendorID        string
	VendorErrorCode string
	UpdatedAt       time.Time
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
	if c.Connectors == nil {
		c.Connectors = make(map[int]Connector)
	}
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
	if c.Connectors == nil {
		c.Connectors = make(map[int]Connector)
	}
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

func (r *Registry) ConnectorStatus(chargerID string, connector Connector, reportedAt time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()

	receivedAt := time.Now().UTC()
	if reportedAt.IsZero() {
		reportedAt = receivedAt
	}
	charger := r.chargers[chargerID]
	charger.ID = chargerID
	if charger.Connectors == nil {
		charger.Connectors = make(map[int]Connector)
	}
	connector.UpdatedAt = reportedAt.UTC()
	charger.Connectors[connector.ID] = connector
	charger.LastSeenAt = receivedAt
	r.chargers[chargerID] = charger
}

func (r *Registry) Get(id string) (Charger, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	c, ok := r.chargers[id]
	c.Connectors = cloneConnectors(c.Connectors)
	return c, ok
}

func cloneConnectors(connectors map[int]Connector) map[int]Connector {
	cloned := make(map[int]Connector, len(connectors))
	for id, connector := range connectors {
		cloned[id] = connector
	}
	return cloned
}
