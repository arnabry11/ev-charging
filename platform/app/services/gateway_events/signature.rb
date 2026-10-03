require "openssl"

module GatewayEvents
  class Signature
    WINDOW = 5.minutes

    def initialize(secret:, timestamp:, signature:, body:, now: Time.now.utc)
      @secret = secret
      @timestamp = timestamp.to_s
      @signature = signature.to_s
      @body = body
      @now = now
    end

    def valid?
      sent_at = Time.iso8601(@timestamp)
      return false if (@now - sent_at).abs > WINDOW

      expected = OpenSSL::HMAC.hexdigest("SHA256", @secret, "#{@timestamp}.#{@body}")
      return false unless expected.bytesize == @signature.bytesize

      ActiveSupport::SecurityUtils.secure_compare(expected, @signature)
    rescue ArgumentError
      false
    end

    private

    attr_reader :secret, :timestamp, :signature, :body, :now
  end
end
