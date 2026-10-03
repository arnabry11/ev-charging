module Internal
  module V1
    class PrepaidSessionsController < ApplicationController
      skip_forgery_protection

      def show
        session = Tenant.poc.prepaid_sessions.find(params[:id])
        render json: { session: session.as_registry_json }
      end

      def create
        result = PrepaidSessions::CreateService.new(
          tenant: Tenant.poc,
          idempotency_key: params[:idempotency_key],
          phone: params[:phone],
          ocpp_id: params[:ocpp_id],
          prepaid_paise: params[:prepaid_paise],
          card: params[:card]
        ).call
        render json: result.payload, status: result.status
      end
    end
  end
end
