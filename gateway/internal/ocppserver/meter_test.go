package ocppserver

import (
	"testing"
	"time"

	"github.com/lorenzodonini/ocpp-go/ocpp1.6/core"
	"github.com/lorenzodonini/ocpp-go/ocpp1.6/types"
)

func TestEnergyReadingUsesLatestSupportedRegister(t *testing.T) {
	t.Parallel()

	earlier := time.Date(2026, time.October, 3, 8, 0, 0, 0, time.UTC)
	later := earlier.Add(time.Minute)
	request := &core.MeterValuesRequest{
		MeterValue: []types.MeterValue{
			{
				Timestamp: types.NewDateTime(later),
				SampledValue: []types.SampledValue{{
					Value:     "10.25",
					Measurand: types.MeasurandEnergyActiveImportRegister,
					Unit:      types.UnitOfMeasureKWh,
				}},
			},
			{
				Timestamp: types.NewDateTime(earlier),
				SampledValue: []types.SampledValue{{
					Value:     "9999",
					Measurand: types.MeasurandEnergyActiveImportRegister,
					Unit:      types.UnitOfMeasureWh,
				}},
			},
		},
	}

	energyWh, recordedAt, ok := energyReading(request)
	if !ok || energyWh != 10_250 || !recordedAt.Equal(later) {
		t.Fatalf("reading = (%d, %s, %t)", energyWh, recordedAt, ok)
	}
}

func TestEnergyReadingRejectsUnsupportedValues(t *testing.T) {
	t.Parallel()

	request := &core.MeterValuesRequest{
		MeterValue: []types.MeterValue{{
			Timestamp: types.NewDateTime(time.Now()),
			SampledValue: []types.SampledValue{
				{Value: "7200", Measurand: types.MeasurandPowerActiveImport, Unit: types.UnitOfMeasureW},
				{Value: "not-a-number", Measurand: types.MeasurandEnergyActiveImportRegister, Unit: types.UnitOfMeasureWh},
			},
		}},
	}

	if _, _, ok := energyReading(request); ok {
		t.Fatal("unsupported samples must not produce an energy reading")
	}
}
