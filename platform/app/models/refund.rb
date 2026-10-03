class Refund < ApplicationRecord
  belongs_to :tenant
  belongs_to :prepaid_session
end
