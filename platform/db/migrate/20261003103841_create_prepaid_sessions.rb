class CreatePrepaidSessions < ActiveRecord::Migration[8.1]
  def change
    create_table :prepaid_sessions, id: :uuid do |t|
      t.references :tenant, null: false, foreign_key: true, type: :uuid
      t.references :driver, null: false, foreign_key: true, type: :uuid
      t.references :charger, null: false, foreign_key: true, type: :uuid
      t.string :idempotency_key, null: false
      t.string :state, null: false
      t.bigint :prepaid_paise, null: false
      t.bigint :energy_price_paise, null: false
      t.bigint :session_fee_paise, null: false
      t.bigint :limit_energy_wh
      t.integer :limit_duration_s
      t.uuid :command_id
      t.string :card_last4, null: false
      t.bigint :meter_start_wh
      t.bigint :meter_stop_wh
      t.bigint :last_energy_wh
      t.datetime :started_at
      t.datetime :stopped_at
      t.timestamps
    end

    add_index :prepaid_sessions, [ :tenant_id, :idempotency_key ], unique: true
    add_index :prepaid_sessions, :command_id, unique: true
    add_check_constraint :prepaid_sessions, "prepaid_paise > 0", name: "prepaid_sessions_prepaid_positive"
    add_check_constraint :prepaid_sessions, "energy_price_paise > 0", name: "prepaid_sessions_energy_price_positive"
    add_check_constraint :prepaid_sessions, "session_fee_paise >= 0", name: "prepaid_sessions_session_fee_non_negative"
    add_check_constraint :prepaid_sessions,
      "state IN ('declined', 'paid', 'start_requested', 'start_failed', 'charging', 'stopped')",
      name: "prepaid_sessions_state"
    add_check_constraint :prepaid_sessions, "card_last4 ~ '^[0-9]{4}$'", name: "prepaid_sessions_card_last4"
    add_check_constraint :prepaid_sessions,
      "state = 'declined' OR (limit_energy_wh > 0 AND limit_duration_s > 0)",
      name: "prepaid_sessions_limits_when_accepted"
  end
end
