module GatewayEvents
  class ReceiveService
    EVENT_TYPES = %w[session.started session.meter_values session.stopped command.result].freeze

    def initialize(payload:)
      @payload = payload
    end

    def call
      return ServiceResponse.error("invalid_event") unless valid_payload?

      result = nil
      ProcessedGatewayEvent.transaction do
        if ProcessedGatewayEvent.exists?(event_id:)
          result = ServiceResponse.success({ status: "duplicate" })
        elsif sequence != next_sequence
          result = ServiceResponse.error("sequence_gap", status: :conflict)
        else
          ProcessedGatewayEvent.create!(attributes)
          PrepaidSessions::ApplyEvent.new(event: payload).call
          result = ServiceResponse.success({ status: "accepted" })
        end
      end
      result
    rescue ActiveRecord::RecordNotUnique
      if ProcessedGatewayEvent.exists?(event_id:)
        ServiceResponse.success({ status: "duplicate" })
      else
        ServiceResponse.error("sequence_gap", status: :conflict)
      end
    end

    private

    attr_reader :payload

    def valid_payload?
      payload.is_a?(Hash) &&
        uuid?(event_id) &&
        uuid?(session_ref) &&
        sequence.is_a?(Integer) &&
        sequence.positive? &&
        EVENT_TYPES.include?(payload["event_type"]) &&
        payload["payload"].is_a?(Hash) &&
        occurred_at.present?
    end

    def next_sequence
      last = ProcessedGatewayEvent.where(session_ref:).order(sequence: :desc).lock("FOR UPDATE").pick(:sequence)
      last.nil? ? 1 : last + 1
    end

    def attributes
      {
        event_id:,
        tenant_id: Tenant::POC_ID,
        session_ref:,
        sequence:,
        event_type: payload.fetch("event_type"),
        payload: payload.fetch("payload"),
        occurred_at:
      }
    end

    def event_id
      payload["event_id"]
    end

    def session_ref
      payload["session_ref"]
    end

    def sequence
      Integer(payload["sequence"])
    rescue ArgumentError, TypeError
      nil
    end

    def occurred_at
      Time.iso8601(payload["occurred_at"].to_s)
    rescue ArgumentError
      nil
    end

    def uuid?(value)
      value.to_s.match?(/\A[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\z/i)
    end
  end
end
