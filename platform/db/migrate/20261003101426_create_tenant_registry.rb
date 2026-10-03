class CreateTenantRegistry < ActiveRecord::Migration[8.1]
  def change
    create_table :tenants, id: :uuid do |t|
      t.string :name, null: false
      t.timestamps
    end

    create_table :chargers, id: :uuid do |t|
      t.references :tenant, null: false, foreign_key: true, type: :uuid
      t.string :ocpp_id, null: false
      t.string :name, null: false
      t.timestamps
    end
    add_index :chargers, [ :tenant_id, :ocpp_id ], unique: true

    create_table :drivers, id: :uuid do |t|
      t.references :tenant, null: false, foreign_key: true, type: :uuid
      t.string :phone, null: false
      t.string :name
      t.timestamps
    end
    add_index :drivers, [ :tenant_id, :phone ], unique: true
  end
end
