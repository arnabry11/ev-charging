require "openssl"
require "rails_helper"

RSpec.describe "Gateway events", type: :request do
  let(:secret) { "dev-gateway-secret" }
  let(:session_ref) { "b3a0f1d2-1111-4444-8888-123456789abc" }

  before do
    ENV["GATEWAY_SIGNING_SECRET"] = secret
  end

  it "accepts a signed event and ignores the duplicate" do
    body = event_body(sequence: 1)

    post_event(body)
    expect(response).to have_http_status(:ok)
    expect(response.parsed_body).to eq("status" => "accepted")

    post_event(body)
    expect(response).to have_http_status(:ok)
    expect(response.parsed_body).to eq("status" => "duplicate")
    expect(ProcessedGatewayEvent.count).to eq(1)
  end

  it "rejects a gap so the publisher retries the missing sequence" do
    post_event(event_body(sequence: 2))

    expect(response).to have_http_status(:conflict)
    expect(response.parsed_body).to eq("error" => "sequence_gap")
    expect(ProcessedGatewayEvent.count).to eq(0)
  end

  it "rejects an invalid signature" do
    post "/internal/v1/gateway-events",
      params: event_body(sequence: 1),
      headers: { "X-Timestamp" => Time.now.utc.iso8601, "X-Signature" => "00" },
      as: :json

    expect(response).to have_http_status(:unauthorized)
  end

  def post_event(body)
    timestamp = Time.now.utc.iso8601
    post "/internal/v1/gateway-events",
      params: body,
      headers: {
        "CONTENT_TYPE" => "application/json",
        "X-Timestamp" => timestamp,
        "X-Signature" => OpenSSL::HMAC.hexdigest("SHA256", secret, "#{timestamp}.#{body}")
      }
  end

  def event_body(sequence:)
    {
      event_id: sequence == 1 ? "6f1c7a52-9c21-4c0e-8e1c-0a9f7d1c2b11" : "7a2d8c63-2e6e-49a6-8732-0a127349a501",
      event_type: "session.started",
      session_ref:,
      sequence:,
      occurred_at: "2026-10-03T09:00:00Z",
      payload: { meter_start_wh: 100_000 }
    }.to_json
  end
end
