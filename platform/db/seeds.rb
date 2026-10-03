tenant = Tenant.find_or_create_by!(id: Tenant::POC_ID) { |record| record.name = "POC" }

# Matches the chargers the simulator runs (SIM_CHARGER_COUNT, default 10).
(1..10).each do |number|
  tenant.chargers.find_or_create_by!(ocpp_id: format("CHG-MUM-%04d", number)) do |charger|
    charger.name = format("Mumbai charger %02d", number)
  end
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
