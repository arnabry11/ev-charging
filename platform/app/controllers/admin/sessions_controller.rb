module Admin
  class SessionsController < BaseController
    def index
      @sessions = Tenant.poc.prepaid_sessions.includes(:charger, :driver, :invoice, :refund).order(created_at: :desc)
    end
  end
end
