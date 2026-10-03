module ApplicationHelper
  def paise_to_rupees(paise)
    return "—" if paise.nil?

    whole, fraction = paise.abs.divmod(100)
    format("%s₹%s.%02d", paise.negative? ? "-" : "", group_indian(whole), fraction)
  end

  # Energy is displayed to the nearest 10 Wh. It is never used to price anything.
  def kwh_value(wh)
    return "—" if wh.nil?

    hundredths = (wh.abs + 5) / 10
    format("%s%d.%02d", wh.negative? ? "-" : "", hundredths / 100, hundredths % 100)
  end

  def wh_to_kwh(wh)
    return "—" if wh.nil?

    "#{kwh_value(wh)} kWh"
  end

  private

  # 1234567 -> "12,34,567": the last three digits, then pairs.
  def group_indian(number)
    digits = number.to_s
    return digits if digits.length <= 3

    head = digits[0...-3]
    "#{head.reverse.scan(/\d{1,2}/).join(",").reverse},#{digits[-3..]}"
  end
end
