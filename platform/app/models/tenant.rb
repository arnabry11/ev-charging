class Tenant < ApplicationRecord
  POC_ID = "00000000-0000-0000-0000-000000000001"

  has_many :chargers, dependent: :restrict_with_exception
  has_many :drivers, dependent: :restrict_with_exception

  validates :name, presence: true

  def self.poc
    find(POC_ID)
  end
end
