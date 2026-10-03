module ApplicationHelper
  def paise_to_rupees(paise)
    return "—" if paise.nil?

    whole, fraction = paise.abs.divmod(100)
    format("%s₹%d.%02d", paise.negative? ? "-" : "", whole, fraction)
  end

  def wh_to_kwh(wh)
    return "—" if wh.nil?

    whole, fraction = wh.abs.divmod(1_000)
    format("%s%d.%03d kWh", wh.negative? ? "-" : "", whole, fraction)
  end

  def chart_coordinates(points, width: 640, height: 180, pad: 28)
    return "" if points.blank?

    times = points.map { |point| point[:at].to_f }
    min_t, max_t = times.minmax
    max_value = points.map { |point| point[:value].to_f }.max
    max_value = 1.0 if max_value <= 0
    span = max_t - min_t
    span = 1.0 if span.zero?
    inner_width = width - (pad * 2)
    inner_height = height - (pad * 2)
    points.map { |point| coordinate(point, min_t, span, max_value, pad, inner_width, inner_height) }.join(" ")
  end

  private

  def coordinate(point, min_t, span, max_value, pad, inner_width, inner_height)
    x = pad + ((point[:at].to_f - min_t) / span * inner_width)
    y = pad + inner_height - (point[:value].to_f / max_value * inner_height)
    format("%.1f,%.1f", x, y)
  end
end
