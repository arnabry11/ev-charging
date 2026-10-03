module PrepaidSessions
  class ApplyEvent
    def initialize(event:)
      @event = event
    end

    def call
      session = PrepaidSession.find_by(id: event["session_ref"])
      return if session.nil?

      payload = event["payload"] || {}
      case event["event_type"]
      when "command.result" then apply_command(session, payload)
      when "session.started" then apply_started(session, payload)
      when "session.meter_values" then apply_meter(session, payload)
      when "session.stopped" then apply_stopped(session, payload)
      end
    end

    private

    attr_reader :event

    def apply_command(session, payload)
      return unless payload["command_type"] == "start_session"
      return unless session.state == "start_requested"
      return unless payload["result"] == "rejected" || payload["state"] == "failed"

      session.update!(state: "start_failed")
    end

    def apply_started(session, payload)
      return unless %w[paid start_requested].include?(session.state)

      session.update!(
        state: "charging",
        meter_start_wh: payload["meter_start_wh"],
        last_energy_wh: payload["meter_start_wh"],
        started_at: payload["started_at"]
      )
    end

    def apply_meter(session, payload)
      return unless session.state == "charging"

      energy = payload["energy_wh"]
      return if energy.nil? || (session.last_energy_wh && energy < session.last_energy_wh)

      session.update!(last_energy_wh: energy)
    end

    def apply_stopped(session, payload)
      return unless %w[charging start_requested].include?(session.state)

      session.update!(
        state: "stopped",
        meter_stop_wh: payload["meter_stop_wh"],
        last_energy_wh: payload["meter_stop_wh"],
        stopped_at: payload["stopped_at"]
      )
    end
  end
end
