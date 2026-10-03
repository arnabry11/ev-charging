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

ActiveRecord::Schema[8.1].define(version: 2026_10_03_092907) do
  # These are extensions that must be enabled in order to support this database
  enable_extension "pg_catalog.plpgsql"

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
end
