# This file is auto-generated from the current state of the database. Instead
# of editing this file, please use the migrations feature of Active Record to
# incrementally modify your database, and then regenerate this schema definition.
#
# This file is the source Rails uses to define your schema when running `bin/rails
# db:schema:load`. When creating a new database, `bin/rails db:schema:load` tends to
# be faster and is potentially less error prone than running all of your
# migrations from scratch. Old migrations may fail to apply correctly if those
# migrations use external dependencies or application code.
#
# It's strongly recommended that you check this file into your version control system.

ActiveRecord::Schema[8.1].define(version: 2026_10_03_105337) do
  # These are extensions that must be enabled in order to support this database
  enable_extension "pg_catalog.plpgsql"

  create_table "chargers", id: :uuid, default: -> { "gen_random_uuid()" }, force: :cascade do |t|
    t.uuid "tenant_id", null: false
    t.string "ocpp_id", null: false
    t.string "name", null: false
    t.datetime "created_at", null: false
    t.datetime "updated_at", null: false
    t.index ["tenant_id", "ocpp_id"], name: "index_chargers_on_tenant_id_and_ocpp_id", unique: true
    t.index ["tenant_id"], name: "index_chargers_on_tenant_id"
  end

  create_table "drivers", id: :uuid, default: -> { "gen_random_uuid()" }, force: :cascade do |t|
    t.uuid "tenant_id", null: false
    t.string "phone", null: false
    t.string "name"
    t.datetime "created_at", null: false
    t.datetime "updated_at", null: false
    t.index ["tenant_id", "phone"], name: "index_drivers_on_tenant_id_and_phone", unique: true
    t.index ["tenant_id"], name: "index_drivers_on_tenant_id"
  end

  create_table "invoices", id: :uuid, default: -> { "gen_random_uuid()" }, force: :cascade do |t|
    t.uuid "tenant_id", null: false
    t.uuid "prepaid_session_id", null: false
    t.bigint "energy_wh", null: false
    t.bigint "taxable_paise", null: false
    t.integer "gst_rate_percent", null: false
    t.bigint "cgst_paise", null: false
    t.bigint "sgst_paise", null: false
    t.bigint "total_paise", null: false
    t.datetime "created_at", null: false
    t.datetime "updated_at", null: false
    t.index ["prepaid_session_id"], name: "index_invoices_on_prepaid_session_id", unique: true
    t.index ["tenant_id"], name: "index_invoices_on_tenant_id"
    t.check_constraint "cgst_paise = sgst_paise OR cgst_paise = (sgst_paise + 1)", name: "invoices_odd_paisa_on_cgst"
    t.check_constraint "energy_wh >= 0", name: "invoices_energy_non_negative"
    t.check_constraint "gst_rate_percent >= 0 AND gst_rate_percent <= 100", name: "invoices_gst_rate"
    t.check_constraint "taxable_paise >= 0 AND cgst_paise >= 0 AND sgst_paise >= 0 AND total_paise >= 0", name: "invoices_amounts_non_negative"
    t.check_constraint "total_paise = (taxable_paise + cgst_paise + sgst_paise)", name: "invoices_total_matches_parts"
  end

  create_table "prepaid_sessions", id: :uuid, default: -> { "gen_random_uuid()" }, force: :cascade do |t|
    t.uuid "tenant_id", null: false
    t.uuid "driver_id", null: false
    t.uuid "charger_id", null: false
    t.string "idempotency_key", null: false
    t.string "state", null: false
    t.bigint "prepaid_paise", null: false
    t.bigint "energy_price_paise", null: false
    t.bigint "session_fee_paise", null: false
    t.bigint "limit_energy_wh"
    t.integer "limit_duration_s"
    t.uuid "command_id"
    t.string "card_last4", null: false
    t.bigint "meter_start_wh"
    t.bigint "meter_stop_wh"
    t.bigint "last_energy_wh"
    t.datetime "started_at"
    t.datetime "stopped_at"
    t.datetime "created_at", null: false
    t.datetime "updated_at", null: false
    t.index ["charger_id"], name: "index_prepaid_sessions_on_charger_id"
    t.index ["command_id"], name: "index_prepaid_sessions_on_command_id", unique: true
    t.index ["driver_id"], name: "index_prepaid_sessions_on_driver_id"
    t.index ["tenant_id", "idempotency_key"], name: "index_prepaid_sessions_on_tenant_id_and_idempotency_key", unique: true
    t.index ["tenant_id"], name: "index_prepaid_sessions_on_tenant_id"
    t.check_constraint "card_last4::text ~ '^[0-9]{4}$'::text", name: "prepaid_sessions_card_last4"
    t.check_constraint "energy_price_paise > 0", name: "prepaid_sessions_energy_price_positive"
    t.check_constraint "prepaid_paise > 0", name: "prepaid_sessions_prepaid_positive"
    t.check_constraint "session_fee_paise >= 0", name: "prepaid_sessions_session_fee_non_negative"
    t.check_constraint "state::text = 'declined'::text OR limit_energy_wh > 0 AND limit_duration_s > 0", name: "prepaid_sessions_limits_when_accepted"
    t.check_constraint "state::text = ANY (ARRAY['declined'::character varying::text, 'paid'::character varying::text, 'start_requested'::character varying::text, 'start_failed'::character varying::text, 'charging'::character varying::text, 'stopped'::character varying::text])", name: "prepaid_sessions_state"
  end

  create_table "processed_gateway_events", primary_key: "event_id", id: :uuid, default: nil, force: :cascade do |t|
    t.uuid "tenant_id", null: false
    t.uuid "session_ref", null: false
    t.bigint "sequence", null: false
    t.string "event_type", null: false
    t.jsonb "payload", null: false
    t.datetime "occurred_at", null: false
    t.datetime "created_at", null: false
    t.datetime "updated_at", null: false
    t.index ["session_ref", "sequence"], name: "index_processed_gateway_events_on_session_ref_and_sequence", unique: true
  end

  create_table "refunds", id: :uuid, default: -> { "gen_random_uuid()" }, force: :cascade do |t|
    t.uuid "tenant_id", null: false
    t.uuid "prepaid_session_id", null: false
    t.bigint "amount_paise", null: false
    t.datetime "created_at", null: false
    t.datetime "updated_at", null: false
    t.index ["prepaid_session_id"], name: "index_refunds_on_prepaid_session_id", unique: true
    t.index ["tenant_id"], name: "index_refunds_on_tenant_id"
    t.check_constraint "amount_paise >= 0", name: "refunds_amount_non_negative"
  end

  create_table "tariffs", id: :uuid, default: -> { "gen_random_uuid()" }, force: :cascade do |t|
    t.uuid "tenant_id", null: false
    t.string "name", null: false
    t.bigint "energy_price_paise", null: false
    t.bigint "session_fee_paise", default: 0, null: false
    t.boolean "active", default: false, null: false
    t.datetime "created_at", null: false
    t.datetime "updated_at", null: false
    t.index ["tenant_id"], name: "index_tariffs_on_tenant_id"
    t.index ["tenant_id"], name: "index_tariffs_one_active_per_tenant", unique: true, where: "active"
    t.check_constraint "energy_price_paise > 0", name: "tariffs_energy_price_positive"
    t.check_constraint "session_fee_paise >= 0", name: "tariffs_session_fee_non_negative"
  end

  create_table "tenants", id: :uuid, default: -> { "gen_random_uuid()" }, force: :cascade do |t|
    t.string "name", null: false
    t.datetime "created_at", null: false
    t.datetime "updated_at", null: false
  end

  add_foreign_key "chargers", "tenants"
  add_foreign_key "drivers", "tenants"
  add_foreign_key "invoices", "prepaid_sessions"
  add_foreign_key "invoices", "tenants"
  add_foreign_key "prepaid_sessions", "chargers"
  add_foreign_key "prepaid_sessions", "drivers"
  add_foreign_key "prepaid_sessions", "tenants"
  add_foreign_key "refunds", "prepaid_sessions"
  add_foreign_key "refunds", "tenants"
  add_foreign_key "tariffs", "tenants"
end
