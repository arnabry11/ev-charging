class AddMeterLookupIndexToProcessedGatewayEvents < ActiveRecord::Migration[8.1]
  def change
    add_index :processed_gateway_events, %i[tenant_id event_type created_at], name: "index_processed_gateway_events_on_tenant_type_received"
  end
end
