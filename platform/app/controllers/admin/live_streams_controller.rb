module Admin
  class LiveStreamsController < BaseController
    include ActionController::Live

    def show
      return head(:service_unavailable) unless LiveStream.acquire

      begin
        stream_board
      ensure
        LiveStream.release
        response.stream.close
      end
    end

    private

    def stream_board
      response.headers["Content-Type"] = "text/event-stream"
      response.headers["Cache-Control"] = "no-cache"
      response.headers["X-Accel-Buffering"] = "no"
      LiveStream.new(render: method(:render_board)).each_frame { |frame| response.stream.write(frame) }
    rescue ActionController::Live::ClientDisconnected, IOError
      nil
    end

    # The stream outlives a request, so hold a database connection only while
    # the board is being built.
    def render_board
      ActiveRecord::Base.connection_pool.with_connection do
        render_to_string(partial: "admin/live/board", locals: { board: LiveBoard.new.call.payload }, formats: [ :html ])
      end
    end
  end
end
