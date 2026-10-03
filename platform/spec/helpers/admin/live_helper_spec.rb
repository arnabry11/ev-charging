require "rails_helper"

RSpec.describe Admin::LiveHelper, type: :helper do
  describe "#power_axis_ceiling" do
    it "rounds the peak up to a tidy number so the axis does not jitter" do
      expect(helper.power_axis_ceiling(0)).to eq(1.0)
      expect(helper.power_axis_ceiling(0.4)).to eq(1.0)
      expect(helper.power_axis_ceiling(7.2)).to eq(10.0)
      expect(helper.power_axis_ceiling(10)).to eq(10.0)
      expect(helper.power_axis_ceiling(14.4)).to eq(20.0)
      expect(helper.power_axis_ceiling(22)).to eq(50.0)
      expect(helper.power_axis_ceiling(52)).to eq(100.0)
      expect(helper.power_axis_ceiling(150)).to eq(200.0)
    end
  end

  describe "#axis_kw" do
    it "drops a trailing .0" do
      expect(helper.axis_kw(20.0)).to eq("20 kW")
      expect(helper.axis_kw(0.5)).to eq("0.5 kW")
      expect(helper.axis_kw(0)).to eq("0 kW")
    end
  end

  describe "#power_chart" do
    let(:points) { [ { at: 0, kw: 0.0 }, { at: 1, kw: 10.0 }, { at: 2, kw: 5.0 } ] }

    it "spreads the points across the plot and scales them to the ceiling" do
      expect(helper.power_chart(points, 10.0)[:line]).to eq("48.0,192.0 376.0,12.0 704.0,102.0")
    end

    it "closes the area down to the floor" do
      expect(helper.power_chart(points, 10.0)[:area]).to eq("48.0,192.0 376.0,12.0 704.0,102.0 704.0,192.0 48.0,192.0")
    end

    it "keeps a value above the ceiling inside the plot" do
      expect(helper.power_chart([ { at: 0, kw: 0.0 }, { at: 1, kw: 99.0 } ], 10.0)[:line]).to eq("48.0,192.0 704.0,12.0")
    end

    it "draws nothing without points" do
      expect(helper.power_chart([], 10.0)).to eq({ line: "", area: "" })
    end
  end

  describe "#power_gridlines" do
    it "labels the ceiling, the middle and the floor" do
      lines = helper.power_gridlines(20.0)

      expect(lines.map { |line| [ line[:label], line[:y] ] }).to eq([ [ "20 kW", 12.0 ], [ "10 kW", 102.0 ], [ "0 kW", 192.0 ] ])
    end
  end
end
