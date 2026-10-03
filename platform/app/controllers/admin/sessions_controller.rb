module Admin
  class SessionsController < BaseController
    def index
      @sessions = Tenant.poc.prepaid_sessions.includes(:charger, :driver, :invoice, :refund).order(created_at: :desc)
    end

    def show
      @session = Tenant.poc.prepaid_sessions.includes(:charger, :driver, :invoice, :refund).find(params[:id])
      @events = ProcessedGatewayEvent.where(tenant_id: Tenant::POC_ID, session_ref: @session.id).order(:sequence)
    end
  end
end
