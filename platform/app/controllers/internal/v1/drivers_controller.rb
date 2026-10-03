module Internal
  module V1
    class DriversController < ApplicationController
      skip_forgery_protection

      def index
        render json: { drivers: Tenant.poc.drivers.ordered.map(&:as_registry_json) }
      end

      def create
        result = Drivers::RegisterService.new(
          tenant: Tenant.poc,
          phone: params[:phone],
          name: params[:name]
        ).call
        render json: result.payload, status: result.status
      end
    end
  end
end
