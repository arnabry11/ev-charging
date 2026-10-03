require "rails_helper"

RSpec.describe "Flat tariff", type: :request do
  before do
    Tenant.create!(id: Tenant::POC_ID, name: "POC")
  end

  it "stores one active tariff in paise" do
    get "/internal/v1/tariff"
    expect(response).to have_http_status(:not_found)

    put "/internal/v1/tariff", params: {
      name: "Flat",
      energy_price_paise: 1_800,
      session_fee_paise: 1_000
    }, as: :json

    expect(response).to have_http_status(:ok)
    expect(response.parsed_body["tariff"]).to include(
      "energy_price_paise" => 1_800,
      "session_fee_paise" => 1_000
    )

    put "/internal/v1/tariff", params: {
      name: "Flat",
      energy_price_paise: "18.50",
      session_fee_paise: 0
    }, as: :json
    expect(response).to have_http_status(:unprocessable_content)
    expect(Tenant.poc.active_tariff.energy_price_paise).to eq(1_800)
  end
end

RSpec.describe "Development seed", type: :model do
  it "is idempotent" do
    2.times { Rails.application.load_seed }

    expect(Tenant.where(id: Tenant::POC_ID).count).to eq(1)
    expect(Charger.order(:ocpp_id).pluck(:ocpp_id)).to eq((1..10).map { |number| format("CHG-MUM-%04d", number) })
    expect(Charger.find_by!(ocpp_id: "CHG-MUM-0001").name).to eq("Mumbai charger 01")
    expect(Driver.find_by!(phone: "9876543210").name).to eq("Demo driver")
    expect(Tenant.poc.active_tariff.energy_price_paise).to eq(1_800)
    expect(Tenant.poc.active_tariff.session_fee_paise).to eq(1_000)
    expect(Tariff.where(active: true).count).to eq(1)
  end
end
