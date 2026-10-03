require "json"
require "net/http"
require "openssl"
require "securerandom"

module PrepaidSessions
  class StartCommand
    def initialize(sender: nil)
      @sender = sender || method(:post)
    end

    def call(session)
      return session unless %w[paid start_requested].include?(session.state)

      session.update!(state: "start_requested", command_id: session.command_id || SecureRandom.uuid)
      deliver(session) if gateway_url.present?
      session.reload
    end

    private

    attr_reader :sender

    def deliver(session)
      body = JSON.generate(command_body(session))
      timestamp = Time.now.utc.iso8601
      sender.call(
        "#{gateway_url}/internal/v1/commands/start-session",
        {
          "Content-Type" => "application/json",
          "X-Timestamp" => timestamp,
          "X-Signature" => OpenSSL::HMAC.hexdigest("SHA256", signing_secret, "#{timestamp}.#{body}")
        },
        body
      )
    rescue StandardError
      session
    end

    def command_body(session)
      {
        command_id: session.command_id,
        session_ref: session.id,
        charger_id: session.charger.ocpp_id,
        connector_id: 1,
        id_tag: session.driver.phone,
        limits: {
          max_energy_wh: session.limit_energy_wh,
          max_duration_s: session.limit_duration_s
        },
        expires_at: 5.minutes.from_now.utc.iso8601
      }
    end

    def post(url, headers, body)
      uri = URI(url)
      http = Net::HTTP.new(uri.host, uri.port)
      http.use_ssl = uri.scheme == "https"
      http.open_timeout = 2
      http.read_timeout = 15
      request = Net::HTTP::Post.new(uri)
      headers.each { |key, value| request[key] = value }
      request.body = body
      response = http.request(request)
      raise "gateway returned #{response.code}" unless response.is_a?(Net::HTTPSuccess) || response.code == "202"
    end

    def gateway_url
      ENV["GATEWAY_URL"].to_s.delete_suffix("/")
    end

    def signing_secret
      ENV.fetch("PLATFORM_SIGNING_SECRET")
    end
  end
end
