class Charger < ApplicationRecord
  OCPP_ID_FORMAT = /\A[A-Z0-9][A-Z0-9_-]{0,47}\z/

  belongs_to :tenant

  validates :ocpp_id, format: { with: OCPP_ID_FORMAT }
  validates :name, presence: true, length: { maximum: 80 }

  scope :ordered, -> { order(:ocpp_id) }

  def as_registry_json
    { id:, ocpp_id:, name: }
  end
end
