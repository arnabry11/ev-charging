require "rails_helper"

RSpec.describe Admin::LiveStream do
  let(:no_sleep) { ->(_seconds) { } }

  def frames_for(renders, max_ticks:, **options)
    queue = renders.dup
    stream = described_class.new(render: -> { queue.shift || renders.last }, max_ticks:, sleeper: no_sleep, **options)
    [].tap { |frames| stream.each_frame { |frame| frames << frame } }
  end

  it "sends the board as a named event and splits multi-line html into data lines" do
    frames = frames_for([ "<p>one</p>\n<p>two</p>" ], max_ticks: 1)

    expect(frames).to eq([ "event: board\ndata: <p>one</p>\ndata: <p>two</p>\n\n" ])
  end

  it "sends again only when the rendered board changed" do
    frames = frames_for([ "a", "a", "b" ], max_ticks: 3, ping_every: 100)

    expect(frames).to eq([ "event: board\ndata: a\n\n", "event: board\ndata: b\n\n" ])
  end

  it "sends a comment ping while the board is unchanged so idle connections stay open" do
    frames = frames_for([ "a" ], max_ticks: 5, ping_every: 2)

    expect(frames).to eq([ "event: board\ndata: a\n\n", ": ping\n\n", ": ping\n\n" ])
  end

  it "waits between ticks" do
    waits = []
    stream = described_class.new(render: -> { "a" }, interval: 2, max_ticks: 3, sleeper: ->(seconds) { waits << seconds })
    stream.each_frame { |_frame| }

    expect(waits).to eq([ 2, 2 ])
  end

  describe ".acquire" do
    before do
      stub_const("Admin::LiveStream::MAX_CONNECTIONS", 2)
      described_class.reset!
    end

    after { described_class.reset! }

    it "hands out a limited number of stream slots and takes a released one back" do
      expect(described_class.acquire).to be(true)
      expect(described_class.acquire).to be(true)
      expect(described_class.acquire).to be(false)

      described_class.release
      expect(described_class.acquire).to be(true)
    end
  end
end
