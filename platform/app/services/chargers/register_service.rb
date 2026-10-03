module Chargers
  class RegisterService
    def initialize(tenant:, ocpp_id:, name:)
      @tenant = tenant
      @ocpp_id = ocpp_id.to_s.strip.upcase
      @name = name.to_s.strip
    end

    def call
      charger = tenant.chargers.new(ocpp_id:, name:)
      return ServiceResponse.error("invalid_charger") unless charger.valid?

      charger.save!
      ServiceResponse.success({ charger: charger.as_registry_json }, status: :created)
    rescue ActiveRecord::RecordNotUnique
      ServiceResponse.error("charger_exists", status: :conflict)
    end

    private

    attr_reader :tenant, :ocpp_id, :name
  end
end
