module Admin
  module LiveHelper
    SESSION_STATES = {
      "charging" => { label: "Charging", tone: "live" },
      "stopped" => { label: "Stopped", tone: "done" },
      "start_requested" => { label: "Starting", tone: "pending" }
    }.freeze
    IDLE_STATE = { label: "Idle", tone: "idle" }.freeze

    # The fleet power chart is drawn in a fixed 720 x 220 box with room for axis labels.
    POWER_CHART = { width: 720, height: 220, left: 48, right: 16, top: 12, bottom: 28 }.freeze

    def session_state(session)
      SESSION_STATES.fetch(session&.state, IDLE_STATE)
    end

    # Separate items rather than one joined string, so the card can space them apart.
    def charger_meta(row)
      state = row[:connection_state]
      [
        { text: row[:charger].ocpp_id },
        { text: state.capitalize, css: "link-state link-#{state.parameterize}" },
        ({ text: "Connector #{row[:connector_status]}" } if row[:connector_status])
      ].compact
    end

    def empty_caption(session)
      session ? "Waiting for the session to start" : "No session yet"
    end

    # The next 1, 2, 5 or 10 times a power of ten at or above the peak, so the axis
    # does not rescale on every reading.
    def power_axis_ceiling(peak_kw)
      return 1.0 if peak_kw.to_f <= 1.0

      magnitude = 10.0**Math.log10(peak_kw).floor
      [ 1, 2, 5, 10 ].map { |factor| factor * magnitude }.find { |ceiling| ceiling >= peak_kw }
    end

    def axis_kw(value)
      value == value.to_i ? "#{value.to_i} kW" : "#{value} kW"
    end

    # Polyline and filled-area points for the fleet power chart.
    def power_chart(points, ceiling)
      return { line: "", area: "" } if points.blank?

      line = points.each_with_index.map do |point, index|
        format("%.1f,%.1f", power_x(index, points.size), power_y(point[:kw], ceiling))
      end
      floor = format("%.1f", power_y(0, ceiling))
      right = format("%.1f", power_x(points.size - 1, points.size))
      left = format("%.1f", power_x(0, points.size))
      { line: line.join(" "), area: [ *line, "#{right},#{floor}", "#{left},#{floor}" ].join(" ") }
    end

    def power_gridlines(ceiling)
      [ ceiling, ceiling / 2, 0 ].map { |value| { label: axis_kw(value), y: power_y(value, ceiling) } }
    end

    def power_plot
      box = POWER_CHART
      { left: box[:left], right: box[:width] - box[:right], top: box[:top], bottom: box[:height] - box[:bottom] }
    end

    private

    def power_x(index, count)
      plot = power_plot
      return plot[:left].to_f if count == 1

      plot[:left] + (index * (plot[:right] - plot[:left]).to_f / (count - 1))
    end

    def power_y(kw, ceiling)
      plot = power_plot
      plot[:top] + ((1 - [ kw.to_f / ceiling, 1.0 ].min) * (plot[:bottom] - plot[:top]))
    end
  end
end
