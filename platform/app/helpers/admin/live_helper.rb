module Admin
  module LiveHelper
    SESSION_STATES = {
      "charging" => { label: "Charging", tone: "live" },
      "stopped" => { label: "Stopped", tone: "done" },
      "start_requested" => { label: "Starting", tone: "pending" }
    }.freeze
    IDLE_STATE = { label: "Idle", tone: "idle" }.freeze

    def session_state(session)
      SESSION_STATES.fetch(session&.state, IDLE_STATE)
    end

    def charger_meta(row)
      connector = ("connector #{row[:connector_status]}" if row[:connector_status])
      [ row[:charger].ocpp_id, row[:connection_state], connector, row[:session]&.driver&.phone ].compact.join(" · ")
    end

    def empty_caption(session)
      session ? "Waiting for the session to start" : "No session yet"
    end
  end
end
