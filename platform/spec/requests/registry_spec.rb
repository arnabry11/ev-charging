require "rails_helper"

RSpec.describe "Platform registry", type: :request do
  before do
    Tenant.create!(id: Tenant::POC_ID, name: "POC")
  end

  it "registers and lists a charger" do
    post "/internal/v1/chargers", params: { ocpp_id: "chg-mum-0001", name: "Mumbai 1" }, as: :json

    expect(response).to have_http_status(:created)
    expect(response.parsed_body.dig("charger", "ocpp_id")).to eq("CHG-MUM-0001")

    post "/internal/v1/chargers", params: { ocpp_id: "CHG-MUM-0001", name: "Again" }, as: :json
    expect(response).to have_http_status(:conflict)

    get "/internal/v1/chargers"
    expect(response.parsed_body["chargers"].map { |charger| charger["ocpp_id"] }).to eq([ "CHG-MUM-0001" ])
  end

  it "rejects an invalid charger id" do
    post "/internal/v1/chargers", params: { ocpp_id: "bad id", name: "Mumbai" }, as: :json

    expect(response).to have_http_status(:unprocessable_content)
  end

  it "normalizes a driver phone and rejects a duplicate" do
    post "/internal/v1/drivers", params: { phone: "+91 98765 43210", name: "Asha" }, as: :json

    expect(response).to have_http_status(:created)
    expect(response.parsed_body.dig("driver", "phone")).to eq("9876543210")

    post "/internal/v1/drivers", params: { phone: "09876543210" }, as: :json
    expect(response).to have_http_status(:conflict)

    get "/internal/v1/drivers"
    expect(response.parsed_body["drivers"].size).to eq(1)
  end

  it "rejects a phone that is not a mobile number" do
    post "/internal/v1/drivers", params: { phone: "12345" }, as: :json

    expect(response).to have_http_status(:unprocessable_content)
  end
end
