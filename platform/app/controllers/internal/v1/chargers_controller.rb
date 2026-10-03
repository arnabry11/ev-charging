module Internal
  module V1
    class ChargersController < ApplicationController
      skip_forgery_protection

      def index
        render json: { chargers: Tenant.poc.chargers.ordered.map(&:as_registry_json) }
      end

      def create
        result = Chargers::RegisterService.new(
          tenant: Tenant.poc,
          ocpp_id: params[:ocpp_id],
          name: params[:name]
        ).call
        render json: result.payload, status: result.status
      end
    end
  end
end
