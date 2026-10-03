module Internal
  module V1
    class TariffsController < ApplicationController
      skip_forgery_protection

      def show
        tariff = Tenant.poc.active_tariff
        if tariff.nil?
          render json: { error: "tariff_missing" }, status: :not_found
          return
        end

        render json: { tariff: tariff.as_registry_json }
      end

      def update
        result = Tariffs::SetActiveService.new(
          tenant: Tenant.poc,
          name: params[:name],
          energy_price_paise: params[:energy_price_paise],
          session_fee_paise: params[:session_fee_paise]
        ).call
        render json: result.payload, status: result.status
      end
    end
  end
end
