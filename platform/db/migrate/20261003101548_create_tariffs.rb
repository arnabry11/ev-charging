class CreateTariffs < ActiveRecord::Migration[8.1]
  def change
    create_table :tariffs, id: :uuid do |t|
      t.references :tenant, null: false, foreign_key: true, type: :uuid
      t.string :name, null: false
      t.bigint :energy_price_paise, null: false
      t.bigint :session_fee_paise, null: false, default: 0
      t.boolean :active, null: false, default: false
      t.timestamps
    end

    add_check_constraint :tariffs, "energy_price_paise > 0", name: "tariffs_energy_price_positive"
    add_check_constraint :tariffs, "session_fee_paise >= 0", name: "tariffs_session_fee_non_negative"
    add_index :tariffs, :tenant_id, unique: true, where: "active", name: "index_tariffs_one_active_per_tenant"
  end
end
