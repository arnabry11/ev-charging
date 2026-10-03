require "rails_helper"

RSpec.describe "Admin lists", type: :request do
  before do
    tenant = Tenant.create!(id: Tenant::POC_ID, name: "POC")
    charger = tenant.chargers.create!(ocpp_id: "CHG-MUM-0001", name: "Mumbai demo charger")
    driver = tenant.drivers.create!(phone: "9876543210", name: "Demo driver")
    tenant.prepaid_sessions.create!(
      driver:,
      charger:,
      idempotency_key: "pay-admin",
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

  it "shows chargers and sessions" do
    get "/"
    expect(response).to have_http_status(:ok)
    expect(response.body).to include("Sessions")
    expect(response.body).to include("9876543210")
    expect(response.body).to include("₹14.32")
    expect(response.body).not_to include("1432")
    expect(response.body).not_to include("paise")

    get "/admin/chargers"
    expect(response).to have_http_status(:ok)
    expect(response.body).to include("CHG-MUM-0001")
    expect(response.body).to include("Mumbai demo charger")
  end

  it "shows an empty session list" do
    PrepaidSession.delete_all

    get "/admin/sessions"
    expect(response.body).to include("No sessions yet.")
  end
end
