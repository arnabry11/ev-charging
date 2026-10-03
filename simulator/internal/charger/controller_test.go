package charger

import (
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/lorenzodonini/ocpp-go/ocpp1.6/core"
	"github.com/lorenzodonini/ocpp-go/ocpp1.6/types"
)

func TestRemoteSessionProducesMeterValuesAndStops(t *testing.T) {
	t.Parallel()

	client := newFakeClient()
	controller := New(client, Config{
		PowerW:                  7_200,
		MeterStartWh:            100_000,
		TickInterval:            time.Millisecond,
		SimulatedSecondsPerTick: 60,
		Logger:                  slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	connectorID := 1
	if got := controller.RemoteStart(&core.RemoteStartTransactionRequest{
		ConnectorId: &connectorID,
		IdTag:       "ID-1",
	}); got != types.RemoteStartStopStatusAccepted {
		t.Fatalf("remote start = %s", got)
	}

	select {
	case meterWh := <-client.meters:
		if meterWh <= 100_000 {
			t.Fatalf("meter = %d", meterWh)
		}
	case <-time.After(time.Second):
		t.Fatal("meter value not sent")
	}

	if got := controller.RemoteStop(&core.RemoteStopTransactionRequest{
		TransactionId: 42,
	}); got != types.RemoteStartStopStatusAccepted {
		t.Fatalf("remote stop = %s", got)
	}
	select {
	case stopped := <-client.stops:
		if stopped.reason != core.ReasonRemote {
			t.Fatalf("stop reason = %s", stopped.reason)
		}
		if stopped.meterWh <= 100_000 {
			t.Fatalf("stop meter = %d", stopped.meterWh)
		}
	case <-time.After(time.Second):
		t.Fatal("stop transaction not sent")
	}
}

func TestRemoteStartRejectsBusyConnector(t *testing.T) {
	t.Parallel()

	client := newFakeClient()
	controller := New(client, Config{
		TickInterval: time.Hour,
		Logger:       slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	request := &core.RemoteStartTransactionRequest{IdTag: "ID-1"}
	if got := controller.RemoteStart(request); got != types.RemoteStartStopStatusAccepted {
		t.Fatalf("first start = %s", got)
	}
	if got := controller.RemoteStart(request); got != types.RemoteStartStopStatusRejected {
		t.Fatalf("second start = %s", got)
	}
	<-client.started
	if got := controller.RemoteStop(&core.RemoteStopTransactionRequest{TransactionId: 42}); got != types.RemoteStartStopStatusAccepted {
		t.Fatalf("remote stop = %s", got)
	}
	<-client.stops
}

func TestNextMeterWhCarriesFractionalWattHours(t *testing.T) {
	t.Parallel()

	meter, remainder := nextMeterWh(1_000, 0, 1_000, 1)
	if meter != 1_000 || remainder != 1_000 {
		t.Fatalf("first tick = (%d, %d)", meter, remainder)
	}
	for range 3 {
		meter, remainder = nextMeterWh(meter, remainder, 1_000, 1)
	}
	if meter != 1_001 || remainder != 400 {
		t.Fatalf("four ticks = (%d, %d)", meter, remainder)
	}
}

type stoppedSession struct {
	meterWh int
	reason  core.Reason
}

type fakeClient struct {
	started chan struct{}
	meters  chan int64
	stops   chan stoppedSession
}

func newFakeClient() *fakeClient {
	return &fakeClient{
		started: make(chan struct{}, 1),
		meters:  make(chan int64, 10),
		stops:   make(chan stoppedSession, 1),
	}
}

func (f *fakeClient) StatusNotification(
	int,
	core.ChargePointErrorCode,
	core.ChargePointStatus,
	...func(*core.StatusNotificationRequest),
) (*core.StatusNotificationConfirmation, error) {
	return core.NewStatusNotificationConfirmation(), nil
}

func (f *fakeClient) StartTransaction(
	_ int,
	_ string,
	_ int,
	_ *types.DateTime,
	_ ...func(*core.StartTransactionRequest),
) (*core.StartTransactionConfirmation, error) {
	f.started <- struct{}{}
	return core.NewStartTransactionConfirmation(
		types.NewIdTagInfo(types.AuthorizationStatusAccepted),
		42,
	), nil
}

func (f *fakeClient) MeterValues(
	_ int,
	values []types.MeterValue,
	props ...func(*core.MeterValuesRequest),
) (*core.MeterValuesConfirmation, error) {
	request := core.NewMeterValuesRequest(1, values)
	for _, prop := range props {
		prop(request)
	}
	value := values[0].SampledValue[0].Value
	var meterWh int64
	for _, digit := range value {
		meterWh = meterWh*10 + int64(digit-'0')
	}
	f.meters <- meterWh
	return core.NewMeterValuesConfirmation(), nil
}

func (f *fakeClient) StopTransaction(
	meterStop int,
	at *types.DateTime,
	transactionID int,
	props ...func(*core.StopTransactionRequest),
) (*core.StopTransactionConfirmation, error) {
	request := core.NewStopTransactionRequest(meterStop, at, transactionID)
	for _, prop := range props {
		prop(request)
	}
	f.stops <- stoppedSession{meterWh: meterStop, reason: request.Reason}
	return core.NewStopTransactionConfirmation(), nil
}
