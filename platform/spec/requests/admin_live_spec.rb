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

  def card
    Nokogiri::HTML.parse(response.body).at_css("article")
  end

  def card_text(selector)
    card.css(selector).map { |node| node.text.squish }
  end

  it "shows the energy, the running amount and the progress for a charging session" do
    charge = charge_session(state: "charging", last_energy_wh: 100_120)
    record_meter_event(charge, sequence: 1, energy_wh: 100_120)

    get "/admin/live"

    expect(response).to have_http_status(:ok)
    expect(card_text("h2")).to eq([ "Mumbai demo charger" ])
    expect(card_text(".state")).to eq([ "Charging" ])
    expect(card_text(".meta > span")).to eq([ "CHG-MUM-0001", "Connected", "Connector Charging", "9876543210" ])
    expect(card_text(".meta .link-state")).to eq([ "Connected" ])
    expect(card_text(".stat-label")).to eq([ "Energy", "Amount so far" ])
    expect(card_text(".stat-value")).to eq([ "0.12 kWh", "₹12.16" ])
    expect(card_text(".stat-sub")).to eq([ "of 0.24 kWh", "of ₹14.32 prepaid" ])
    expect(card.at_css("[role=progressbar]")["aria-valuenow"]).to eq("50")
    expect(card_text(".tax")).to eq([ "Taxable ₹10.31 · CGST ₹0.93 · SGST ₹0.92" ])
    expect(response.body).to include('data-stream-url="/admin/live/stream"')
    expect(response.body).not_to include('http-equiv="refresh"')
    expect(card.at_css("svg")).to be_nil
    expect(response.body).not_to include("4242424242424242")
  end

  it "shows an idle charger when the gateway cannot be reached" do
    charger
    allow(gateway).to receive(:fetch).and_return(nil)

    get "/admin/live"

    expect(card_text(".state")).to eq([ "Idle" ])
    expect(card_text(".meta > span")).to eq([ "CHG-MUM-0001", "Unknown" ])
    expect(card_text(".stat-value")).to eq([ "—", "—" ])
    expect(card_text(".stat-sub")).to eq([ "No session yet", "No session yet" ])
    expect(card.at_css("[role=progressbar]")["aria-valuenow"]).to eq("0")
  end

  def charge_session(state:, **attributes)
    tenant.prepaid_sessions.create!({
      driver: tenant.drivers.find_or_create_by!(phone: "9876543210"),
      charger:,
      idempotency_key: SecureRandom.uuid,
      state:,
      prepaid_paise: 1_432,
      energy_price_paise: 1_800,
      session_fee_paise: 1_000,
      limit_energy_wh: 240,
      limit_duration_s: 5_400,
      card_last4: "4242",
      meter_start_wh: 100_000,
      started_at: Time.utc(2026, 10, 3, 11, 0, 0)
    }.merge(attributes))
  end

  def record_meter_event(session_record, sequence:, energy_wh:)
    ProcessedGatewayEvent.create!(
      event_id: SecureRandom.uuid,
      tenant_id: Tenant::POC_ID,
      session_ref: session_record.id,
      sequence:,
      event_type: "session.meter_values",
      payload: { "energy_wh" => energy_wh },
      occurred_at: Time.utc(2026, 10, 3, 11, 0, sequence)
    )
  end

  it "keeps the final numbers and a full progress bar after a session stops" do
    charge_session(state: "stopped", last_energy_wh: 100_240, meter_stop_wh: 100_240, stopped_at: Time.utc(2026, 10, 3, 11, 0, 3))

    get "/admin/live"

    expect(card_text(".state")).to eq([ "Stopped" ])
    expect(card_text(".stat-label")).to eq([ "Energy", "Final amount" ])
    expect(card_text(".stat-value")).to eq([ "0.24 kWh", "₹14.32" ])
    expect(card.at_css("[role=progressbar]")["aria-valuenow"]).to eq("100")
  end

  it "leaves a stopped session out of the live totals" do
    charge_session(state: "stopped", last_energy_wh: 100_240, meter_stop_wh: 100_240, stopped_at: Time.utc(2026, 10, 3, 11, 0, 3))

    get "/admin/live"

    totals = Nokogiri::HTML.parse(response.body).css(".live-totals .stat-value").map { |node| node.text.squish }
    expect(totals).to eq([ "0", "0.00 kWh", "₹0.00" ])
  end

  it "adds the live sessions up in the totals" do
    charge_session(state: "charging", last_energy_wh: 100_120)

    get "/admin/live"

    totals = Nokogiri::HTML.parse(response.body).css(".live-totals .stat-value").map { |node| node.text.squish }
    expect(totals).to eq([ "1", "0.12 kWh", "₹12.16" ])
  end

  it "shows the newest session when a charger has run more than one" do
    charge_session(state: "stopped", last_energy_wh: 100_240, meter_stop_wh: 100_240, created_at: 1.hour.ago, stopped_at: 1.hour.ago)
    charge_session(state: "charging", last_energy_wh: 100_060, created_at: 1.minute.ago, driver: tenant.drivers.create!(phone: "9123456780"))

    get "/admin/live"

    expect(card_text(".state")).to eq([ "Charging" ])
    expect(card_text(".meta").first).to include("9123456780")
  end

  it "shows a charger waiting for its session to start" do
    charge_session(state: "start_requested", meter_start_wh: nil)

    get "/admin/live"

    expect(card_text(".state")).to eq([ "Starting" ])
    expect(card_text(".stat-sub")).to eq([ "Waiting for the session to start", "Waiting for the session to start" ])
  end

  describe "fleet power" do
    let(:fleet_power) do
      {
        points: [ { at: Time.utc(2026, 10, 3, 11, 50), kw: 0.0 }, { at: Time.utc(2026, 10, 3, 11, 55), kw: 14.4 }, { at: Time.utc(2026, 10, 3, 12, 0), kw: 7.2 } ],
        current_kw: 7.2,
        peak_kw: 14.4,
        window_s: 600
      }
    end

    before do
      charger
      allow(Admin::FleetPower).to receive(:new).and_return(instance_double(Admin::FleetPower, call: fleet_power))
    end

    def fleet_card
      Nokogiri::HTML.parse(response.body).at_css(".fleet-power")
    end

    it "shows the current and peak power with a chart of the last ten minutes" do
      get "/admin/live"

      expect(fleet_card.at_css("h2").text).to eq("Fleet power")
      expect(fleet_card.css(".stat-value").map { |node| node.text.squish }).to eq([ "7.20 kW", "14.40 kW" ])
      expect(fleet_card.css(".stat-label").map(&:text)).to eq([ "Now", "Peak" ])
      expect(fleet_card.css(".axis").map(&:text)).to eq([ "20 kW", "10 kW", "0 kW", "10 min ago", "5 min ago", "now" ])
      expect(fleet_card.at_css("polyline.power-line")["points"]).to be_present
      expect(fleet_card.at_css("polygon.power-area")["points"]).to be_present
    end

    it "sits above the charger cards" do
      get "/admin/live"

      body = response.body
      expect(body.index('class="fleet-power"')).to be < body.index('class="live-grid"')
    end
  end
end
