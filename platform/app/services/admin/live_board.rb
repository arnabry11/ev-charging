module Admin
  class LiveBoard
    OPEN_STATES = %w[charging start_requested].freeze

    def initialize(tenant: Tenant.poc, gateway: GatewayCharger.new, gst_rate_percent: ENV.fetch("GST_RATE_PERCENT", 18).to_i)
      @tenant = tenant
      @gateway = gateway
      @gst_rate_percent = gst_rate_percent
    end

    def call
      sessions = open_sessions
      events = meter_events(sessions.values.map(&:id))
      chargers = tenant.chargers.ordered.map { |charger| charger_row(charger, sessions[charger.id], events) }
      ServiceResponse.success({
        chargers:,
        charging_count: chargers.count { |row| row[:session]&.state == "charging" },
        delivered_wh: chargers.sum { |row| row[:delivered_wh].to_i },
        running_paise: chargers.sum { |row| row.dig(:quote, :total_paise).to_i }
      })
    end

    private

    attr_reader :tenant, :gateway, :gst_rate_percent

    def charger_row(charger, session, events)
      status = gateway.fetch(charger.ocpp_id)
      quote = running_quote(session)
      series = series_for(session, events[session&.id] || [])
      {
        charger:,
        connection_state: status&.dig("connection_state") || "unknown",
        connector_status: status&.dig("connectors", 0, "status"),
        session:,
        delivered_wh: quote&.dig(:energy_wh),
        quote:,
        energy_series: series.map { |point| { at: point[:at], value: point[:delivered_wh] } },
        amount_series: series.map { |point| { at: point[:at], value: point[:total_paise] } }
      }
    end

    def open_sessions
      tenant.prepaid_sessions.includes(:driver).where(state: OPEN_STATES).order(created_at: :desc).each_with_object({}) do |session, found|
        found[session.charger_id] ||= session
      end
    end

    def meter_events(session_ids)
      return {} if session_ids.empty?

      ProcessedGatewayEvent.where(tenant_id: tenant.id, session_ref: session_ids, event_type: "session.meter_values").order(:sequence).group_by(&:session_ref)
    end

    # Prices the latest register against meter_start, using the settlement
    # formula. The result is a display estimate and is not stored as an invoice.
    def running_quote(session)
      return unless session&.state == "charging"
      return unless session.meter_start_wh && session.last_energy_wh
      return unless gst_rate_percent.between?(0, 100)

      price(session, session.last_energy_wh)
    end

    def series_for(session, events)
      return [] unless session&.state == "charging" && session.meter_start_wh && gst_rate_percent.between?(0, 100)

      points = []
      points << point_at(session, session.meter_start_wh, session.started_at) if session.started_at
      events.each do |event|
        energy = integer_or_nil(event.payload["energy_wh"])
        next if energy.nil?

        points << point_at(session, energy, event.occurred_at)
      end
      append_latest(points, session)
      points.last(120)
    end

    def append_latest(points, session)
      return unless session.last_energy_wh

      latest = point_at(session, session.last_energy_wh, session.updated_at)
      return if points.last && points.last[:delivered_wh] == latest[:delivered_wh]

      points << latest
    end

    def point_at(session, register_wh, at)
      priced = price(session, register_wh)
      { at:, delivered_wh: priced[:energy_wh], total_paise: priced[:total_paise] }
    end

    def price(session, register_wh)
      Billing::Price.calculate(
        meter_start_wh: session.meter_start_wh,
        meter_stop_wh: register_wh,
        energy_price_paise: session.energy_price_paise,
        session_fee_paise: session.session_fee_paise,
        prepaid_paise: session.prepaid_paise,
        gst_rate_percent:
      )
    end

    def integer_or_nil(value)
      Integer(value)
    rescue ArgumentError, TypeError
      nil
    end
  end
end
