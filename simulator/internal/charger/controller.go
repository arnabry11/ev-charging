package charger

import (
	"log/slog"
	"strconv"
	"sync"
	"time"

	"github.com/lorenzodonini/ocpp-go/ocpp1.6/core"
	"github.com/lorenzodonini/ocpp-go/ocpp1.6/types"
)

const (
	stateAvailable = "available"
	stateStarting  = "starting"
	stateCharging  = "charging"
	stateStopping  = "stopping"
)

type Config struct {
	ConnectorID             int
	PowerW                  int64
	MeterStartWh            int64
	TickInterval            time.Duration
	SimulatedSecondsPerTick int64
	Logger                  *slog.Logger
}

type Client interface {
	StatusNotification(int, core.ChargePointErrorCode, core.ChargePointStatus, ...func(*core.StatusNotificationRequest)) (*core.StatusNotificationConfirmation, error)
	StartTransaction(int, string, int, *types.DateTime, ...func(*core.StartTransactionRequest)) (*core.StartTransactionConfirmation, error)
	MeterValues(int, []types.MeterValue, ...func(*core.MeterValuesRequest)) (*core.MeterValuesConfirmation, error)
	StopTransaction(int, *types.DateTime, int, ...func(*core.StopTransactionRequest)) (*core.StopTransactionConfirmation, error)
}

type Controller struct {
	client Client
	cfg    Config

	mu            sync.Mutex
	state         string
	transactionID int
	stop          chan core.Reason
}

func New(client Client, cfg Config) *Controller {
	if cfg.ConnectorID <= 0 {
		cfg.ConnectorID = 1
	}
	if cfg.PowerW <= 0 {
		cfg.PowerW = 7_200
	}
	if cfg.MeterStartWh < 0 {
		cfg.MeterStartWh = 0
	}
	if cfg.TickInterval <= 0 {
		cfg.TickInterval = time.Second
	}
	if cfg.SimulatedSecondsPerTick <= 0 {
		cfg.SimulatedSecondsPerTick = 60
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	return &Controller{
		client: client,
		cfg:    cfg,
		state:  stateAvailable,
	}
}

func (c *Controller) RemoteStart(request *core.RemoteStartTransactionRequest) types.RemoteStartStopStatus {
	connectorID := c.cfg.ConnectorID
	if request.ConnectorId != nil {
		connectorID = *request.ConnectorId
	}
	c.mu.Lock()
	if c.state != stateAvailable || connectorID != c.cfg.ConnectorID {
		c.mu.Unlock()
		return types.RemoteStartStopStatusRejected
	}
	c.state = stateStarting
	c.stop = make(chan core.Reason, 1)
	stop := c.stop
	c.mu.Unlock()

	go c.runSession(request.IdTag, stop)
	return types.RemoteStartStopStatusAccepted
}

func (c *Controller) RemoteStop(request *core.RemoteStopTransactionRequest) types.RemoteStartStopStatus {
	c.mu.Lock()
	defer c.mu.Unlock()

	if request.TransactionId != c.transactionID || (c.state != stateCharging && c.state != stateStopping) {
		return types.RemoteStartStopStatusRejected
	}
	if c.state == stateCharging {
		c.state = stateStopping
		c.stop <- core.ReasonRemote
	}
	return types.RemoteStartStopStatusAccepted
}

func (c *Controller) runSession(idTag string, stop <-chan core.Reason) {
	startedAt := time.Now().UTC()
	if !c.notify(core.ChargePointStatusPreparing, startedAt) {
		c.reset()
		return
	}
	confirmation, err := c.current().StartTransaction(
		c.cfg.ConnectorID,
		idTag,
		int(c.cfg.MeterStartWh),
		types.NewDateTime(startedAt),
	)
	if err != nil || confirmation.IdTagInfo.Status != types.AuthorizationStatusAccepted {
		c.cfg.Logger.Error("StartTransaction", "err", err)
		c.resetAvailable(time.Now().UTC())
		return
	}

	c.mu.Lock()
	c.transactionID = confirmation.TransactionId
	c.state = stateCharging
	c.mu.Unlock()
	if !c.notify(core.ChargePointStatusCharging, startedAt) {
		c.finishSession(c.cfg.MeterStartWh, startedAt, core.ReasonOther)
		return
	}

	ticker := time.NewTicker(c.cfg.TickInterval)
	defer ticker.Stop()
	meterWh := c.cfg.MeterStartWh
	simulatedAt := startedAt
	var remainder int64
	for {
		select {
		case reason := <-stop:
			c.finishSession(meterWh, simulatedAt, reason)
			return
		case <-ticker.C:
			simulatedAt = simulatedAt.Add(time.Duration(c.cfg.SimulatedSecondsPerTick) * time.Second)
			meterWh, remainder = nextMeterWh(meterWh, remainder, c.cfg.PowerW, c.cfg.SimulatedSecondsPerTick)
			if err := c.sendMeterValue(meterWh, simulatedAt); err != nil {
				c.cfg.Logger.Error("MeterValues", "err", err)
			}
		}
	}
}

func (c *Controller) sendMeterValue(meterWh int64, at time.Time) error {
	c.mu.Lock()
	transactionID := c.transactionID
	c.mu.Unlock()
	_, err := c.current().MeterValues(
		c.cfg.ConnectorID,
		[]types.MeterValue{{
			Timestamp: types.NewDateTime(at),
			SampledValue: []types.SampledValue{{
				Value:     strconv.FormatInt(meterWh, 10),
				Context:   types.ReadingContextSamplePeriodic,
				Measurand: types.MeasurandEnergyActiveImportRegister,
				Unit:      types.UnitOfMeasureWh,
			}},
		}},
		func(request *core.MeterValuesRequest) {
			request.TransactionId = &transactionID
		},
	)
	return err
}

func (c *Controller) finishSession(meterWh int64, at time.Time, reason core.Reason) {
	_ = c.notify(core.ChargePointStatusFinishing, at)
	c.mu.Lock()
	transactionID := c.transactionID
	c.mu.Unlock()
	if _, err := c.current().StopTransaction(
		int(meterWh),
		types.NewDateTime(at),
		transactionID,
		func(request *core.StopTransactionRequest) {
			request.Reason = reason
		},
	); err != nil {
		c.cfg.Logger.Error("StopTransaction", "err", err)
	}
	c.resetAvailable(at)
}

func (c *Controller) resetAvailable(at time.Time) {
	c.reset()
	_ = c.notify(core.ChargePointStatusAvailable, at)
}

func (c *Controller) reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.state = stateAvailable
	c.transactionID = 0
	c.stop = nil
}

func (c *Controller) Use(client Client) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.client = client
}

func (c *Controller) ConnectorStatus() core.ChargePointStatus {
	c.mu.Lock()
	defer c.mu.Unlock()
	switch c.state {
	case stateStarting:
		return core.ChargePointStatusPreparing
	case stateCharging:
		return core.ChargePointStatusCharging
	case stateStopping:
		return core.ChargePointStatusFinishing
	default:
		return core.ChargePointStatusAvailable
	}
}

func (c *Controller) current() Client {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.client
}

func (c *Controller) notify(status core.ChargePointStatus, at time.Time) bool {
	_, err := c.current().StatusNotification(
		c.cfg.ConnectorID,
		core.NoError,
		status,
		func(request *core.StatusNotificationRequest) {
			request.Timestamp = types.NewDateTime(at)
		},
	)
	if err != nil {
		c.cfg.Logger.Error("StatusNotification", "status", status, "err", err)
		return false
	}
	return true
}

func nextMeterWh(currentWh, remainder, powerW, seconds int64) (int64, int64) {
	total := remainder + powerW*seconds
	return currentWh + total/3_600, total % 3_600
}
