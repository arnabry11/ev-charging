tenant = Tenant.find_or_create_by!(id: Tenant::POC_ID) { |record| record.name = "POC" }

tenant.chargers.find_or_create_by!(ocpp_id: "CHG-MUM-0001") do |charger|
  charger.name = "Mumbai demo charger"
end

tenant.drivers.find_or_create_by!(phone: "9876543210") do |driver|
  driver.name = "Demo driver"
end

Tariffs::SetActiveService.new(
  tenant:,
  name: "Flat",
  energy_price_paise: 1_800,
  session_fee_paise: 1_000
).call
