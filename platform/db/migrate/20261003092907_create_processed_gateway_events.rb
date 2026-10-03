class CreateProcessedGatewayEvents < ActiveRecord::Migration[8.1]
  def change
    create_table :processed_gateway_events, id: false do |t|
      t.uuid :event_id, null: false, primary_key: true
      t.uuid :tenant_id, null: false
      t.uuid :session_ref, null: false
      t.bigint :sequence, null: false
      t.string :event_type, null: false
      t.jsonb :payload, null: false
      t.datetime :occurred_at, null: false
      t.timestamps
    end

    add_index :processed_gateway_events, [ :session_ref, :sequence ], unique: true
  end
end
