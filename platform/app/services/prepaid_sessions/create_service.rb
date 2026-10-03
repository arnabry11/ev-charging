module PrepaidSessions
  class CreateService
    def initialize(tenant:, idempotency_key:, phone:, ocpp_id:, prepaid_paise:, card:, max_duration_s: ENV.fetch("SESSION_MAX_DURATION_S", 5_400).to_i)
      @tenant = tenant
      @idempotency_key = idempotency_key.to_s.strip
      @phone = Driver.normalize_phone(phone)
      @ocpp_id = ocpp_id.to_s.strip.upcase
      @prepaid_paise = integer(prepaid_paise)
      @card = card || {}
      @max_duration_s = max_duration_s
    end

    def call
      return ServiceResponse.error("invalid_session") if idempotency_key.blank? || prepaid_paise.nil?

      existing = tenant.prepaid_sessions.find_by(idempotency_key:)
      return replay(existing) if existing

      driver = tenant.drivers.find_by(phone:)
      charger = tenant.chargers.find_by(ocpp_id:)
      tariff = tenant.active_tariff
      return ServiceResponse.error("unknown_driver", status: :not_found) if driver.nil?
      return ServiceResponse.error("unknown_charger", status: :not_found) if charger.nil?
      return ServiceResponse.error("tariff_missing", status: :not_found) if tariff.nil?

      charge = Payments::CardCharge.new(
        number: card[:number] || card["number"],
        expiry_month: card[:expiry_month] || card["expiry_month"],
        expiry_year: card[:expiry_year] || card["expiry_year"],
        cvv: card[:cvv] || card["cvv"]
      ).call
      return charge unless charge.status == :ok

      if charge.payload[:declined]
        session = create_session!(driver:, charger:, tariff:, last4: charge.payload.fetch(:last4), limits: nil)
        return ServiceResponse.success({ session: session.as_registry_json }, status: :created)
      end

      limits = Limits.calculate(
        prepaid_paise:,
        energy_price_paise: tariff.energy_price_paise,
        session_fee_paise: tariff.session_fee_paise,
        max_duration_s:
      )
      return ServiceResponse.error("insufficient_prepaid") if limits.nil?

      session = create_session!(driver:, charger:, tariff:, last4: charge.payload.fetch(:last4), limits:)
      StartCommand.new.call(session)
      ServiceResponse.success({ session: session.reload.as_registry_json }, status: :created)
    rescue ActiveRecord::RecordNotUnique
      replay(tenant.prepaid_sessions.find_by!(idempotency_key:))
    end

    private

    attr_reader :tenant, :idempotency_key, :phone, :ocpp_id, :prepaid_paise, :card, :max_duration_s

    def replay(existing)
      last4 = card_last4
      same = existing.driver.phone == phone &&
        existing.charger.ocpp_id == ocpp_id &&
        existing.prepaid_paise == prepaid_paise &&
        last4.present? &&
        existing.card_last4 == last4
      return ServiceResponse.error("idempotency_conflict", status: :conflict) unless same

      StartCommand.new.call(existing) if %w[paid start_requested].include?(existing.state)
      ServiceResponse.success({ session: existing.reload.as_registry_json })
    end

    def create_session!(driver:, charger:, tariff:, last4:, limits:)
      tenant.prepaid_sessions.create!(
        driver:,
        charger:,
        idempotency_key:,
        state: limits.nil? ? "declined" : "paid",
        prepaid_paise:,
        energy_price_paise: tariff.energy_price_paise,
        session_fee_paise: tariff.session_fee_paise,
        limit_energy_wh: limits&.fetch(:max_energy_wh),
        limit_duration_s: limits&.fetch(:max_duration_s),
        card_last4: last4
      )
    end

    def card_last4
      charge = Payments::CardCharge.new(
        number: card[:number] || card["number"],
        expiry_month: card[:expiry_month] || card["expiry_month"],
        expiry_year: card[:expiry_year] || card["expiry_year"],
        cvv: card[:cvv] || card["cvv"]
      ).call
      charge.payload[:last4]
    end

    def integer(value)
      return value if value.is_a?(Integer)
      return nil unless value.to_s.match?(/\A\d+\z/)

      Integer(value, 10)
    end
  end
end
