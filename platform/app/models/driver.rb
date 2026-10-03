class Driver < ApplicationRecord
  PHONE_FORMAT = /\A[6-9]\d{9}\z/

  belongs_to :tenant

  validates :phone, format: { with: PHONE_FORMAT }
  validates :name, length: { maximum: 80 }, allow_blank: true

  scope :ordered, -> { order(:phone) }

  def self.normalize_phone(value)
    digits = value.to_s.gsub(/\D/, "")
    digits = digits.delete_prefix("91") if digits.length == 12 && digits.start_with?("91")
    digits = digits.delete_prefix("0") if digits.length == 11 && digits.start_with?("0")
    digits
  end

  def as_registry_json
    { id:, phone:, name: }
  end
end
