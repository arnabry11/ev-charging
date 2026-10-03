require "rails_helper"

RSpec.describe "Admin live stream", type: :request do
  let(:gateway) do
    instance_double(
      Admin::GatewayCharger,
      fetch: { "connection_state" => "connected", "connectors" => [ { "status" => "Available" } ] }
    )
  end
  let(:tenant) { Tenant.create!(id: Tenant::POC_ID, name: "POC") }

  before do
    tenant.chargers.create!(ocpp_id: "CHG-MUM-0001", name: "Mumbai charger 01")
    allow(Admin::GatewayCharger).to receive(:new).and_return(gateway)
    Admin::LiveStream.reset!
  end

  after { Admin::LiveStream.reset! }

  it "streams the rendered board as server-sent events" do
    stub_const("Admin::LiveStream::DEFAULT_MAX_TICKS", 1)

    get "/admin/live/stream"

    expect(response).to have_http_status(:ok)
    expect(response.media_type).to eq("text/event-stream")
    expect(response.headers["Cache-Control"]).to include("no-cache")
    expect(response.body).to start_with("event: board\n")
    expect(response.body).to include("data: ")
    expect(response.body).to include("Mumbai charger 01")
    expect(response.body).to end_with("\n\n")
  end

  it "frees its slot when the stream ends" do
    stub_const("Admin::LiveStream::DEFAULT_MAX_TICKS", 1)
    stub_const("Admin::LiveStream::MAX_CONNECTIONS", 1)

    2.times do
      get "/admin/live/stream"
      expect(response).to have_http_status(:ok)
    end
  end

  it "refuses a new stream when every slot is taken" do
    stub_const("Admin::LiveStream::MAX_CONNECTIONS", 1)
    Admin::LiveStream.acquire

    get "/admin/live/stream"

    expect(response).to have_http_status(:service_unavailable)
  end
end
