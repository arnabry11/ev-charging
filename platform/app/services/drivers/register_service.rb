module Drivers
  class RegisterService
    def initialize(tenant:, phone:, name: nil)
      @tenant = tenant
      @phone = Driver.normalize_phone(phone)
      @name = name.to_s.strip.presence
    end

    def call
      driver = tenant.drivers.new(phone:, name:)
      return ServiceResponse.error("invalid_driver") unless driver.valid?

      driver.save!
      ServiceResponse.success({ driver: driver.as_registry_json }, status: :created)
    rescue ActiveRecord::RecordNotUnique
      ServiceResponse.error("driver_exists", status: :conflict)
    end

    private

    attr_reader :tenant, :phone, :name
  end
end
