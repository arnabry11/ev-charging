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

    # Separate items rather than one joined string, so the card can space them apart.
    def charger_meta(row)
      state = row[:connection_state]
      [
        { text: row[:charger].ocpp_id },
        { text: state.capitalize, css: "link-state link-#{state.parameterize}" },
        ({ text: "Connector #{row[:connector_status]}" } if row[:connector_status]),
        ({ text: row[:session].driver.phone } if row[:session])
      ].compact
    end

    def empty_caption(session)
      session ? "Waiting for the session to start" : "No session yet"
    end
  end
end
