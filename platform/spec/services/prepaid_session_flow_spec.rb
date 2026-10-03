require "rails_helper"

RSpec.describe PrepaidSessions::StartCommand, type: :model do
  it "signs the gateway start command with the stored limits" do
    tenant = Tenant.create!(id: Tenant::POC_ID, name: "POC")
    charger = tenant.chargers.create!(ocpp_id: "CHG-MUM-0001", name: "Mumbai")
    driver = tenant.drivers.create!(phone: "9876543210")
    session = tenant.prepaid_sessions.create!(
      driver:,
      charger:,
      idempotency_key: "pay-start",
      state: "paid",
      prepaid_paise: 1_432,
      energy_price_paise: 1_800,
      session_fee_paise: 1_000,
      limit_energy_wh: 240,
      limit_duration_s: 5_400,
      card_last4: "4242"
    )
    sent = nil
    sender = lambda { |_url, headers, body|
      sent = [ headers, body ]
      true
    }
    previous_url = ENV["GATEWAY_URL"]
    previous_secret = ENV["PLATFORM_SIGNING_SECRET"]
    ENV["GATEWAY_URL"] = "http://gateway:8080"
    ENV["PLATFORM_SIGNING_SECRET"] = "secret"
    described_class.new(sender:).call(session)
    ENV["GATEWAY_URL"] = previous_url
    ENV["PLATFORM_SIGNING_SECRET"] = previous_secret

    headers, body = sent
    timestamp = headers.fetch("X-Timestamp")
    expect(headers.fetch("X-Signature")).to eq(
      OpenSSL::HMAC.hexdigest("SHA256", "secret", "#{timestamp}.#{body}")
    )
    expect(JSON.parse(body).dig("limits", "max_energy_wh")).to eq(240)
    expect(session.reload.state).to eq("start_requested")
  end
end

RSpec.describe PrepaidSessions::ApplyEvent, type: :model do
  let(:tenant) { Tenant.create!(id: Tenant::POC_ID, name: "POC") }
  let(:session) do
    tenant.prepaid_sessions.create!(
      driver: tenant.drivers.create!(phone: "9876543210"),
      charger: tenant.chargers.create!(ocpp_id: "CHG-MUM-0001", name: "Mumbai"),
      idempotency_key: "pay-event",
      state: "start_requested",
      prepaid_paise: 1_432,
      energy_price_paise: 1_800,
      session_fee_paise: 1_000,
      limit_energy_wh: 240,
      limit_duration_s: 5_400,
      command_id: SecureRandom.uuid,
      card_last4: "4242"
    )
  end

  it "moves a started session to charging and then stopped" do
    described_class.new(event: {
      "session_ref" => session.id,
      "event_type" => "session.started",
      "payload" => { "meter_start_wh" => 100_000, "started_at" => "2026-10-03T10:00:00Z" }
    }).call
    described_class.new(event: {
      "session_ref" => session.id,
      "event_type" => "session.stopped",
      "payload" => { "meter_stop_wh" => 100_240, "stopped_at" => "2026-10-03T10:02:00Z" }
    }).call

    expect(session.reload.state).to eq("stopped")
    expect(session.meter_stop_wh).to eq(100_240)
  end
end
