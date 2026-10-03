class Invoice < ApplicationRecord
  belongs_to :tenant
  belongs_to :prepaid_session

  def as_registry_json
    {
      id:,
      energy_wh:,
      taxable_paise:,
      gst_rate_percent:,
      cgst_paise:,
      sgst_paise:,
      total_paise:,
      refund_paise: prepaid_session.refund&.amount_paise
    }
  end
end
