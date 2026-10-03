module Admin
  class GatewayCharger
    def fetch(ocpp_id)
      base = ENV["GATEWAY_URL"].to_s.delete_suffix("/")
      return if base.blank?

      uri = URI("#{base}/internal/v1/chargers/#{ocpp_id}")
      response = Net::HTTP.start(uri.host, uri.port, use_ssl: uri.scheme == "https", open_timeout: 1, read_timeout: 1) do |http|
        http.get(uri.request_uri)
      end
      return unless response.is_a?(Net::HTTPSuccess)

      JSON.parse(response.body)
    rescue StandardError
      nil
    end
  end
end
