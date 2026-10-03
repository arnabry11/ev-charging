class Tariff < ApplicationRecord
  belongs_to :tenant

  validates :name, presence: true, length: { maximum: 80 }
  validates :energy_price_paise, numericality: { only_integer: true, greater_than: 0 }
  validates :session_fee_paise, numericality: { only_integer: true, greater_than_or_equal_to: 0 }

  def as_registry_json
    { id:, name:, energy_price_paise:, session_fee_paise: }
  end
end
