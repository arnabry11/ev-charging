module Admin
  class LiveController < BaseController
    def show
      @board = LiveBoard.new.call.payload
    end
  end
end
