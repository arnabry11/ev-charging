module Billing
  class Price
    # Tariff amounts are GST-inclusive. Intra-state tax is split into CGST and
    # SGST; an odd paisa stays on CGST.
    def self.calculate(meter_start_wh:, meter_stop_wh:, energy_price_paise:, session_fee_paise:, prepaid_paise:, gst_rate_percent:)
      delivered = [ meter_stop_wh - meter_start_wh, 0 ].max
      energy_paise = (delivered * energy_price_paise) / 1_000
      inclusive = [ session_fee_paise + energy_paise, prepaid_paise ].min
      gst = inclusive_gst(inclusive, gst_rate_percent)
      cgst = (gst + 1) / 2
      sgst = gst / 2

      {
        energy_wh: delivered,
        taxable_paise: inclusive - gst,
        gst_rate_percent:,
        cgst_paise: cgst,
        sgst_paise: sgst,
        total_paise: inclusive,
        refund_paise: prepaid_paise - inclusive
      }
    end

    def self.inclusive_gst(inclusive, rate)
      divisor = 100 + rate
      (inclusive * rate + divisor / 2) / divisor
    end
    private_class_method :inclusive_gst
  end
end
