require "rails_helper"

RSpec.describe ApplicationHelper, type: :helper do
  describe "#paise_to_rupees" do
    it "shows rupees and paise" do
      expect(helper.paise_to_rupees(1_432)).to eq("₹14.32")
      expect(helper.paise_to_rupees(5)).to eq("₹0.05")
      expect(helper.paise_to_rupees(0)).to eq("₹0.00")
    end

    it "groups digits the Indian way" do
      expect(helper.paise_to_rupees(99_999_00)).to eq("₹99,999.00")
      expect(helper.paise_to_rupees(123_456_789)).to eq("₹12,34,567.89")
    end

    it "keeps the sign and shows a dash for nothing" do
      expect(helper.paise_to_rupees(-250)).to eq("-₹2.50")
      expect(helper.paise_to_rupees(nil)).to eq("—")
    end
  end

  describe "#wh_to_kwh" do
    it "rounds to two decimals" do
      expect(helper.wh_to_kwh(120)).to eq("0.12 kWh")
      expect(helper.wh_to_kwh(2_777)).to eq("2.78 kWh")
      expect(helper.wh_to_kwh(12_345)).to eq("12.35 kWh")
      expect(helper.wh_to_kwh(0)).to eq("0.00 kWh")
    end

    it "shows a dash for nothing" do
      expect(helper.wh_to_kwh(nil)).to eq("—")
      expect(helper.kwh_value(nil)).to eq("—")
    end
  end
end
