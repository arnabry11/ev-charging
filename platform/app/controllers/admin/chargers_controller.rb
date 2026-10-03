module Admin
  class ChargersController < BaseController
    def index
      @chargers = Tenant.poc.chargers.ordered
    end
  end
end
