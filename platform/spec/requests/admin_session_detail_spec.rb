require "rails_helper"

RSpec.describe "Admin session detail", type: :request do
  let(:tenant) { Tenant.create!(id: Tenant::POC_ID, name: "POC") }
  let(:session_record) do
    tenant.prepaid_sessions.create!(
      driver: tenant.drivers.create!(phone: "9876543210"),
      charger: tenant.chargers.create!(ocpp_id: "CHG-MUM-0001", name: "Mumbai"),
      idempotency_key: "pay-detail",
      state: "stopped",
      prepaid_paise: 1_432,
      energy_price_paise: 1_800,
      session_fee_paise: 1_000,
      limit_energy_wh: 240,
      limit_duration_s: 5_400,
      card_last4: "4242",
      meter_start_wh: 100_000,
      meter_stop_wh: 100_240
    )
  end

  before do
    PrepaidSessions::SettleService.new(session_id: session_record.id, gst_rate_percent: 18).call
    ProcessedGatewayEvent.create!(
      event_id: SecureRandom.uuid,
      tenant_id: Tenant::POC_ID,
      session_ref: session_record.id,
      sequence: 1,
      event_type: "session.stopped",
      payload: { "meter_stop_wh" => 100_240 },
      occurred_at: Time.utc(2026, 10, 3, 10, 2, 0)
    )
  end

  it "shows the payment, invoice, refund, and event timeline" do
    get "/admin/sessions/#{session_record.id}"

    expect(response).to have_http_status(:ok)
    expect(response.body).to include("Card ending 4242")
    text = Nokogiri::HTML.parse(response.body).text.squish
    expect(text).to include("Prepaid ₹14.32")
    expect(text).to include("Energy 0.24 kWh")
    expect(text).to include("Taxable ₹12.14")
    expect(text).to include("CGST ₹1.09")
    expect(text).to include("SGST ₹1.09")
    expect(text).to include("Total ₹14.32")
    expect(text).to include("Refund ₹0.00")
    expect(response.body).not_to include("paise")
    expect(response.body).to include("session.stopped")
    expect(response.body).to include("Receipt")
    expect(response.body).not_to include("4242424242424242")
  end
end
