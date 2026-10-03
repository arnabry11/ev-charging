require "rails_helper"

RSpec.describe "Admin live board", type: :request do
  let(:gateway) do
    instance_double(
      Admin::GatewayCharger,
      fetch: { "connection_state" => "connected", "connectors" => [ { "status" => "Charging" } ] }
    )
  end
  let(:tenant) { Tenant.create!(id: Tenant::POC_ID, name: "POC") }
  let(:charger) { tenant.chargers.create!(ocpp_id: "CHG-MUM-0001", name: "Mumbai demo charger") }

  before do
    allow(Admin::GatewayCharger).to receive(:new).and_return(gateway)
  end

  it "shows the running amount and meter graphs for a charging session" do
    session_record = tenant.prepaid_sessions.create!(
      driver: tenant.drivers.create!(phone: "9876543210"),
      charger:,
      idempotency_key: "pay-live",
      state: "charging",
      prepaid_paise: 1_432,
      energy_price_paise: 1_800,
      session_fee_paise: 1_000,
      limit_energy_wh: 240,
      limit_duration_s: 5_400,
      card_last4: "4242",
      meter_start_wh: 100_000,
      last_energy_wh: 100_120,
      started_at: Time.utc(2026, 10, 3, 11, 0, 0)
    )
    ProcessedGatewayEvent.create!(
      event_id: SecureRandom.uuid,
      tenant_id: Tenant::POC_ID,
      session_ref: session_record.id,
      sequence: 1,
      event_type: "session.meter_values",
      payload: { "energy_wh" => 100_120 },
      occurred_at: Time.utc(2026, 10, 3, 11, 0, 1)
    )

    get "/admin/live"

    expect(response).to have_http_status(:ok)
    expect(response.body).to include("Mumbai demo charger")
    expect(response.body).to include("connected")
    expect(response.body).to include("Charging")
    expect(response.body).to include("9876543210")
    expect(response.body).to include("0.120 kWh")
    expect(response.body).to include("0.240 kWh")
    expect(response.body).to include("₹12.16")
    expect(response.body).to include("₹14.32")
    expect(response.body).to include("<polyline")
    expect(response.body).to include('http-equiv="refresh"')
    expect(response.body).not_to include("4242424242424242")
  end

  it "shows an idle charger when the gateway cannot be reached" do
    charger
    allow(gateway).to receive(:fetch).and_return(nil)

    get "/admin/live"

    expect(response.body).to include("unknown")
    expect(response.body).to include("No live session.")
    expect(response.body).to include("₹0.00")
    expect(response.body).to include("0.000 kWh")
  end
end
