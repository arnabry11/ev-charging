module Payments
  class CardCharge
    # The full card number and CVV are checked and discarded. Only the last four
    # digits are returned for storage.
    def initialize(number:, expiry_month:, expiry_year:, cvv:, now: Date.current)
      @number = number.to_s.gsub(/\D/, "")
      @expiry_month = integer(expiry_month)
      @expiry_year = integer(expiry_year)
      @cvv = cvv.to_s
      @now = now
    end

    def call
      return ServiceResponse.error("invalid_card") unless valid?
      return ServiceResponse.success({ last4: number.last(4), declined: true }) if number.end_with?("0002")

      ServiceResponse.success({ last4: number.last(4), declined: false })
    end

    private

    attr_reader :number, :expiry_month, :expiry_year, :cvv, :now

    def valid?
      number.match?(/\A\d{16}\z/) &&
        expiry_month&.between?(1, 12) &&
        expiry_year&.positive? &&
        !expired? &&
        cvv.match?(/\A\d{3}\z/)
    end

    def expired?
      expiry_year < now.year || (expiry_year == now.year && expiry_month < now.month)
    end

    def integer(value)
      return value if value.is_a?(Integer)
      return nil unless value.to_s.match?(/\A\d+\z/)

      Integer(value, 10)
    end
  end
end
