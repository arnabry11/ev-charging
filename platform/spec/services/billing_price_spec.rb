require "rails_helper"

RSpec.describe Billing::Price do
  it "keeps the price at the prepaid energy limit within the prepaid amount" do
    40.times do
      price = rand(1..5_000)
      fee = rand(0..2_000)
      prepaid = fee + rand(price..price * 30)
      duration = rand(60..7_200)
      limits = PrepaidSessions::Limits.calculate(
        prepaid_paise: prepaid,
        energy_price_paise: price,
        session_fee_paise: fee,
        max_duration_s: duration
      )
      next if limits.nil?

      priced = described_class.calculate(
        meter_start_wh: 10_000,
        meter_stop_wh: 10_000 + limits[:max_energy_wh],
        energy_price_paise: price,
        session_fee_paise: fee,
        prepaid_paise: prepaid,
        gst_rate_percent: 18
      )

      expect(priced[:total_paise]).to be <= prepaid
      expect(priced[:total_paise] + priced[:refund_paise]).to eq(prepaid)
      expect(priced[:cgst_paise] + priced[:sgst_paise] + priced[:taxable_paise]).to eq(priced[:total_paise])
      expect(priced[:cgst_paise]).to eq(priced[:sgst_paise]).or eq(priced[:sgst_paise] + 1)
    end
  end

  it "puts an odd paisa on CGST" do
    priced = described_class.calculate(
      meter_start_wh: 0,
      meter_stop_wh: 0,
      energy_price_paise: 1_800,
      session_fee_paise: 4,
      prepaid_paise: 4,
      gst_rate_percent: 18
    )

    expect(priced[:cgst_paise]).to eq(1)
    expect(priced[:sgst_paise]).to eq(0)
    expect(priced[:total_paise] + priced[:refund_paise]).to eq(4)
  end
end

RSpec.describe PrepaidSessions::SettleService, type: :model do
  let(:tenant) { Tenant.create!(id: Tenant::POC_ID, name: "POC") }
  let(:session) do
    tenant.prepaid_sessions.create!(
      driver: tenant.drivers.create!(phone: "9876543210"),
      charger: tenant.chargers.create!(ocpp_id: "CHG-MUM-0001", name: "Mumbai"),
      idempotency_key: "pay-settle",
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

  it "issues one invoice and refunds nothing when the limit is fully used" do
    first = described_class.new(session_id: session.id, gst_rate_percent: 18).call
    second = described_class.new(session_id: session.id, gst_rate_percent: 18).call

    expect(first.status).to eq(:created)
    expect(second.payload.fetch(:invoice).fetch(:id)).to eq(first.payload.fetch(:invoice).fetch(:id))
    expect(Invoice.count).to eq(1)
    expect(Refund.count).to eq(1)
    invoice = first.payload.fetch(:invoice)
    expect(invoice[:total_paise] + invoice[:refund_paise]).to eq(1_432)
    expect(invoice[:refund_paise]).to eq(0)
    expect(invoice[:cgst_paise] + invoice[:sgst_paise] + invoice[:taxable_paise]).to eq(invoice[:total_paise])
  end
end
