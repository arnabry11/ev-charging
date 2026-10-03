require "rails_helper"

RSpec.describe "Prepaid card sessions", type: :request do
  let(:card) { { number: "4242424242424242", expiry_month: 12, expiry_year: 2030, cvv: "123" } }

  before do
    tenant = Tenant.create!(id: Tenant::POC_ID, name: "POC")
    tenant.chargers.create!(ocpp_id: "CHG-MUM-0001", name: "Mumbai demo charger")
    tenant.drivers.create!(phone: "9876543210", name: "Demo driver")
    tenant.tariffs.create!(name: "Flat", energy_price_paise: 1_800, session_fee_paise: 1_000, active: true)
  end

  it "charges a card and stores only the last four digits" do
    post "/internal/v1/prepaid-sessions", params: session_params, as: :json

    expect(response).to have_http_status(:created)
    body = response.parsed_body["session"]
    expect(body).to include(
      "state" => "start_requested",
      "card_last4" => "4242",
      "prepaid_paise" => 1_432,
      "limit_energy_wh" => 240,
      "limit_duration_s" => 5_400
    )
    expect(response.body).not_to include("4242424242424242")
    expect(PrepaidSession.last.card_last4).to eq("4242")

    post "/internal/v1/prepaid-sessions", params: session_params, as: :json
    expect(response).to have_http_status(:ok)
    expect(response.parsed_body.dig("session", "id")).to eq(body["id"])
    expect(PrepaidSession.count).to eq(1)
  end

  it "records a declined card without a gateway limit" do
    post "/internal/v1/prepaid-sessions", params: session_params(card: card.merge(number: "4242424242420002")), as: :json

    expect(response).to have_http_status(:created)
    expect(response.parsed_body.dig("session", "state")).to eq("declined")
    expect(response.parsed_body.dig("session", "limit_energy_wh")).to be_nil
  end

  it "rejects an expired card and a prepaid amount that cannot buy energy" do
    post "/internal/v1/prepaid-sessions", params: session_params(card: card.merge(expiry_year: 2020)), as: :json
    expect(response).to have_http_status(:unprocessable_content)
    expect(response.parsed_body["error"]).to eq("invalid_card")

    post "/internal/v1/prepaid-sessions", params: session_params(prepaid_paise: 1_000), as: :json
    expect(response).to have_http_status(:unprocessable_content)
    expect(response.parsed_body["error"]).to eq("insufficient_prepaid")
    expect(PrepaidSession.count).to eq(0)
  end

  def session_params(prepaid_paise: 1_432, card: self.card)
    {
      idempotency_key: "pay-1",
      phone: "9876543210",
      ocpp_id: "CHG-MUM-0001",
      prepaid_paise:,
      card:
    }
  end
end

RSpec.describe PrepaidSessions::Limits do
  it "keeps the energy priced at the limit within the prepaid amount" do
    30.times do
      price = rand(1..5_000)
      fee = rand(0..2_000)
      prepaid = fee + rand(price..price * 20)
      limits = described_class.calculate(
        prepaid_paise: prepaid,
        energy_price_paise: price,
        session_fee_paise: fee,
        max_duration_s: 3_600
      )
      next if limits.nil?

      cost = fee + (limits[:max_energy_wh] * price) / 1_000
      expect(cost).to be <= prepaid
      expect(limits[:max_energy_wh]).to be >= 1
    end
  end
end
