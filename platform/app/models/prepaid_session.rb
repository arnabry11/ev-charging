class PrepaidSession < ApplicationRecord
  STATES = %w[declined paid start_requested start_failed charging stopped].freeze

  belongs_to :tenant
  belongs_to :driver
  belongs_to :charger
  has_one :invoice, dependent: :restrict_with_exception
  has_one :refund, dependent: :restrict_with_exception

  validates :idempotency_key, presence: true
  validates :state, inclusion: { in: STATES }
  validates :card_last4, format: { with: /\A\d{4}\z/ }
  validates :prepaid_paise, numericality: { only_integer: true, greater_than: 0 }

  def as_registry_json
    {
      id:,
      state:,
      prepaid_paise:,
      card_last4:,
      ocpp_id: charger.ocpp_id,
      phone: driver.phone,
      limit_energy_wh:,
      limit_duration_s:,
      meter_start_wh:,
      meter_stop_wh:
    }
  end
end
