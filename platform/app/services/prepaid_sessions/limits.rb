module PrepaidSessions
  class Limits
    def self.calculate(prepaid_paise:, energy_price_paise:, session_fee_paise:, max_duration_s:)
      return nil unless [ prepaid_paise, energy_price_paise, session_fee_paise, max_duration_s ].all?(Integer)
      return nil if energy_price_paise <= 0 || session_fee_paise.negative? || max_duration_s <= 0
      return nil if prepaid_paise <= session_fee_paise

      energy_wh = ((prepaid_paise - session_fee_paise) * 1_000) / energy_price_paise
      return nil if energy_wh < 1

      { max_energy_wh: energy_wh, max_duration_s: max_duration_s }
    end
  end
end
