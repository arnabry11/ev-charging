module Tariffs
  class SetActiveService
    def initialize(tenant:, name:, energy_price_paise:, session_fee_paise:)
      @tenant = tenant
      @name = name.to_s.strip
      @energy_price_paise = integer_value(energy_price_paise)
      @session_fee_paise = integer_value(session_fee_paise)
    end

    def call
      tariff = tenant.tariffs.find_or_initialize_by(active: true)
      tariff.assign_attributes(name:, energy_price_paise:, session_fee_paise:, active: true)
      return ServiceResponse.error("invalid_tariff") unless tariff.valid?

      tariff.save!
      ServiceResponse.success({ tariff: tariff.as_registry_json })
    rescue ActiveRecord::RecordNotUnique
      ServiceResponse.error("tariff_conflict", status: :conflict)
    end

    private

    attr_reader :tenant, :name, :energy_price_paise, :session_fee_paise

    def integer_value(value)
      return value if value.is_a?(Integer)
      return nil unless value.to_s.match?(/\A\d+\z/)

      Integer(value, 10)
    end
  end
end
