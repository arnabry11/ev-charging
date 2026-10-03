module Admin
  class LiveBoard
    # The board shows each charger's newest session, so a finished session and
    # its final numbers stay on screen until the charger runs another one.
    SHOWN_STATES = %w[start_requested charging stopped].freeze

    def initialize(tenant: Tenant.poc, gateway: GatewayCharger.new, gst_rate_percent: ENV.fetch("GST_RATE_PERCENT", 18).to_i)
      @tenant = tenant
      @gateway = gateway
      @gst_rate_percent = gst_rate_percent
    end

    def call
      sessions = latest_sessions
      chargers = tenant.chargers.ordered.map { |charger| charger_row(charger, sessions[charger.id]) }
      charging = chargers.select { |row| row[:session]&.state == "charging" }
      ServiceResponse.success({
        chargers:,
        charging_count: charging.size,
        delivered_wh: charging.sum { |row| row[:delivered_wh].to_i },
        running_paise: charging.sum { |row| row.dig(:quote, :total_paise).to_i }
      })
    end

    private

    attr_reader :tenant, :gateway, :gst_rate_percent

    def charger_row(charger, session)
      status = gateway.fetch(charger.ocpp_id)
      quote = session_quote(session)
      {
        charger:,
        connection_state: status&.dig("connection_state") || "unknown",
        connector_status: status&.dig("connectors", 0, "status"),
        session:,
        quote:,
        delivered_wh: quote&.dig(:energy_wh),
        progress_percent: progress_percent(session, quote)
      }
    end

    def latest_sessions
      tenant.prepaid_sessions
            .where(state: SHOWN_STATES)
            .select("DISTINCT ON (prepaid_sessions.charger_id) prepaid_sessions.*")
            .order(:charger_id, created_at: :desc)
            .includes(:driver)
            .index_by(&:charger_id)
    end

    # The meter register to show: the latest reading while charging, and the
    # stop reading once the session has stopped.
    def register_wh(session)
      case session&.state
      when "charging" then session.last_energy_wh
      when "stopped" then session.meter_stop_wh
      end
    end

    # Prices the register against meter_start, using the settlement formula.
    # The result is a display estimate and is not stored as an invoice.
    def session_quote(session)
      register = register_wh(session)
      return unless register && session.meter_start_wh
      return unless gst_rate_percent.between?(0, 100)

      Billing::Price.calculate(
        meter_start_wh: session.meter_start_wh,
        meter_stop_wh: register,
        energy_price_paise: session.energy_price_paise,
        session_fee_paise: session.session_fee_paise,
        prepaid_paise: session.prepaid_paise,
        gst_rate_percent:
      )
    end

    def progress_percent(session, quote)
      return 0 unless quote && session.limit_energy_wh.to_i.positive?

      [ quote[:energy_wh] * 100 / session.limit_energy_wh, 100 ].min
    end
  end
end
