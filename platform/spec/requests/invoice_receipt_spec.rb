require "rails_helper"

RSpec.describe "Invoice receipt", type: :request do
  it "renders the settled paise amounts" do
    tenant = Tenant.create!(id: Tenant::POC_ID, name: "POC")
    session = tenant.prepaid_sessions.create!(
      driver: tenant.drivers.create!(phone: "9876543210"),
      charger: tenant.chargers.create!(ocpp_id: "CHG-MUM-0001", name: "Mumbai"),
      idempotency_key: "pay-receipt",
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
    PrepaidSessions::SettleService.new(session_id: session.id, gst_rate_percent: 18).call

    get "/internal/v1/prepaid-sessions/#{session.id}/invoice"

    expect(response).to have_http_status(:ok)
    expect(response.media_type).to eq("text/html")
    expect(response.body).to include('data-amount="total">1432')
    expect(response.body).to include('data-amount="refund">0')
    expect(response.body).to include("4242")
    expect(response.body).not_to include("4242424242424242")
  end
end
