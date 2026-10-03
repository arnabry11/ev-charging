module Admin
  # Produces server-sent event frames for the live board. It is a plain
  # enumerator of frames so the controller owns the socket and the tests own the clock.
  class LiveStream
    # Each open stream holds a Puma thread for its whole life, so the number of
    # streams is capped below the thread count.
    MAX_CONNECTIONS = ENV.fetch("LIVE_STREAM_MAX_CONNECTIONS", 8).to_i
    DEFAULT_MAX_TICKS = nil

    @slots = 0
    @lock = Mutex.new

    class << self
      def acquire
        @lock.synchronize do
          return false if @slots >= MAX_CONNECTIONS

          @slots += 1
          true
        end
      end

      def release
        @lock.synchronize { @slots -= 1 if @slots.positive? }
      end

      def reset!
        @lock.synchronize { @slots = 0 }
      end
    end

    def initialize(render:, interval: 1, ping_every: 15, max_ticks: DEFAULT_MAX_TICKS, sleeper: Kernel.method(:sleep))
      @render = render
      @interval = interval
      @ping_every = ping_every
      @max_ticks = max_ticks
      @sleeper = sleeper
    end

    # Yields a board event when the rendered board changes, and a comment ping
    # after ping_every unchanged ticks so proxies do not close an idle stream.
    def each_frame
      last = nil
      idle = 0
      ticks = 0
      loop do
        html = render.call
        if html != last
          last = html
          idle = 0
          yield board_frame(html)
        else
          idle += 1
          if (idle % ping_every).zero?
            yield ": ping\n\n"
          end
        end
        ticks += 1
        break if max_ticks && ticks >= max_ticks

        sleeper.call(interval)
      end
    end

    private

    attr_reader :render, :interval, :ping_every, :max_ticks, :sleeper

    def board_frame(html)
      data = html.lines(chomp: true).map { |line| "data: #{line}\n" }.join
      "event: board\n#{data}\n"
    end
  end
end
