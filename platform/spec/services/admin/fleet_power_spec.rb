require "rails_helper"

RSpec.describe Admin::FleetPower do
  let(:tenant) { Tenant.create!(id: Tenant::POC_ID, name: "POC") }
  let(:now) { Time.utc(2026, 10, 3, 12, 0, 0) }
  let(:session_a) { SecureRandom.uuid }
  let(:session_b) { SecureRandom.uuid }

  # Receive times are real; occurred_at is the charger's own clock. A charger
  # that reports 20 Wh per 10 charger-seconds is drawing 7.2 kW however fast
  # its clock runs relative to ours.
  def meter_event(session_ref, sequence:, energy_wh:, received_at:, charger_clock:, event_type: "session.meter_values")
    ProcessedGatewayEvent.create!(
      event_id: SecureRandom.uuid,
      tenant_id: tenant.id,
      session_ref:,
      sequence:,
      event_type:,
      payload: { "energy_wh" => energy_wh },
      occurred_at: Time.utc(2026, 10, 3, 9, 0, 0) + charger_clock,
      created_at: received_at
    )
  end

  def power(window: 60, step: 2)
    described_class.new(tenant:, now:, window:, step:).call
  end

  it "turns consecutive meter readings into power using the charger's clock" do
    meter_event(session_a, sequence: 1, energy_wh: 100_000, received_at: now - 10, charger_clock: 0)
    meter_event(session_a, sequence: 2, energy_wh: 100_020, received_at: now - 9, charger_clock: 10)
    meter_event(session_a, sequence: 3, energy_wh: 100_040, received_at: now - 8, charger_clock: 20)

    result = power

    expect(result[:points].map { |point| point[:kw] }.reject(&:zero?)).to eq([ 7.2, 7.2 ])
    expect(result[:peak_kw]).to eq(7.2)
  end

  it "adds up chargers that are drawing power at the same time" do
    [ session_a, session_b ].each do |session_ref|
      meter_event(session_ref, sequence: 1, energy_wh: 100_000, received_at: now - 5, charger_clock: 0)
      meter_event(session_ref, sequence: 2, energy_wh: 100_020, received_at: now - 4, charger_clock: 10)
    end

    result = power

    expect(result[:peak_kw]).to eq(14.4)
    expect(result[:points].map { |point| point[:kw] }.max).to eq(14.4)
  end

  it "fills the quiet seconds with zero so a stopped fleet drops to the floor" do
    meter_event(session_a, sequence: 1, energy_wh: 100_000, received_at: now - 40, charger_clock: 0)
    meter_event(session_a, sequence: 2, energy_wh: 100_020, received_at: now - 39, charger_clock: 10)

    result = power(window: 60, step: 2)

    expect(result[:points].size).to eq(30)
    expect(result[:points].last[:kw]).to eq(0.0)
    expect(result[:current_kw]).to eq(0.0)
    expect(result[:points].first[:at]).to eq(now - 60)
  end

  it "reports the latest bucket as the current power" do
    meter_event(session_a, sequence: 1, energy_wh: 100_000, received_at: now - 3, charger_clock: 0)
    meter_event(session_a, sequence: 2, energy_wh: 100_020, received_at: now - 2, charger_clock: 10)

    expect(power[:current_kw]).to eq(7.2)
  end

  it "ignores other events, other tenants and readings outside the window" do
    meter_event(session_a, sequence: 1, energy_wh: 100_000, received_at: now - 3, charger_clock: 0, event_type: "session.started")
    meter_event(session_a, sequence: 2, energy_wh: 100_020, received_at: now - 2, charger_clock: 10, event_type: "session.stopped")
    meter_event(session_b, sequence: 1, energy_wh: 100_000, received_at: now - 600, charger_clock: 0)
    meter_event(session_b, sequence: 2, energy_wh: 100_020, received_at: now - 599, charger_clock: 10)

    result = power(window: 60)

    expect(result[:peak_kw]).to eq(0.0)
    expect(result[:points].map { |point| point[:kw] }.uniq).to eq([ 0.0 ])
  end

  it "skips readings whose charger clock did not advance" do
    meter_event(session_a, sequence: 1, energy_wh: 100_000, received_at: now - 3, charger_clock: 10)
    meter_event(session_a, sequence: 2, energy_wh: 100_020, received_at: now - 2, charger_clock: 10)

    expect(power[:peak_kw]).to eq(0.0)
  end
end
