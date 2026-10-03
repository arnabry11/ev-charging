module Admin
  # Total power drawn by the fleet over the last few minutes, derived from the
  # meter readings already stored, so it needs no extra storage and survives a restart.
  #
  # Power for one reading is the energy since the previous reading divided by the
  # time between them on the charger's own clock (occurred_at). That is right for a
  # simulator running faster than real time and for a real charger. The time axis is
  # when the platform received the reading.
  class FleetPower
    SQL = <<~SQL.squish.freeze
      WITH readings AS (
        SELECT session_ref, created_at, occurred_at,
               (payload->>'energy_wh')::bigint AS energy_wh,
               LAG((payload->>'energy_wh')::bigint) OVER w AS prev_wh,
               LAG(occurred_at) OVER w AS prev_at
        FROM processed_gateway_events
        WHERE tenant_id = :tenant_id
          AND event_type = 'session.meter_values'
          AND created_at >= CAST(:from AS timestamp)
          AND created_at < CAST(:to AS timestamp)
        WINDOW w AS (PARTITION BY session_ref ORDER BY sequence)
      ), per_session AS (
        SELECT floor(extract(epoch FROM (created_at - CAST(:from AS timestamp))) / :step)::int AS bucket,
               session_ref,
               avg((energy_wh - prev_wh) * 3600.0 / extract(epoch FROM (occurred_at - prev_at))) AS watts
        FROM readings
        WHERE prev_wh IS NOT NULL AND occurred_at > prev_at
        GROUP BY 1, 2
      )
      SELECT bucket, sum(watts) AS watts FROM per_session GROUP BY bucket
    SQL

    def initialize(tenant: Tenant.poc, now: Time.current, window: 600, step: 2)
      @tenant = tenant
      @now = now
      @window = window
      @step = step
    end

    def call
      from = now - window
      watts = watts_by_bucket(from)
      points = Array.new(window / step) do |index|
        { at: from + (index * step), kw: ((watts[index] || 0) / 1000.0).round(2) }
      end
      { points:, current_kw: points.last[:kw], peak_kw: points.map { |point| point[:kw] }.max, window_s: window }
    end

    private

    attr_reader :tenant, :now, :window, :step

    def watts_by_bucket(from)
      sql = ApplicationRecord.sanitize_sql_array([ SQL, { tenant_id: tenant.id, from:, to: now, step: } ])
      ApplicationRecord.connection.select_all(sql).to_h { |row| [ row["bucket"], row["watts"].to_f ] }
    end
  end
end
