class CreateInvoicesAndRefunds < ActiveRecord::Migration[8.1]
  def change
    create_table :invoices, id: :uuid do |t|
      t.references :tenant, null: false, foreign_key: true, type: :uuid
      t.references :prepaid_session, null: false, foreign_key: true, type: :uuid, index: { unique: true }
      t.bigint :energy_wh, null: false
      t.bigint :taxable_paise, null: false
      t.integer :gst_rate_percent, null: false
      t.bigint :cgst_paise, null: false
      t.bigint :sgst_paise, null: false
      t.bigint :total_paise, null: false
      t.timestamps
    end

    add_check_constraint :invoices, "energy_wh >= 0", name: "invoices_energy_non_negative"
    add_check_constraint :invoices, "taxable_paise >= 0 AND cgst_paise >= 0 AND sgst_paise >= 0 AND total_paise >= 0", name: "invoices_amounts_non_negative"
    add_check_constraint :invoices, "gst_rate_percent BETWEEN 0 AND 100", name: "invoices_gst_rate"
    add_check_constraint :invoices, "total_paise = taxable_paise + cgst_paise + sgst_paise", name: "invoices_total_matches_parts"
    add_check_constraint :invoices, "cgst_paise = sgst_paise OR cgst_paise = sgst_paise + 1", name: "invoices_odd_paisa_on_cgst"

    create_table :refunds, id: :uuid do |t|
      t.references :tenant, null: false, foreign_key: true, type: :uuid
      t.references :prepaid_session, null: false, foreign_key: true, type: :uuid, index: { unique: true }
      t.bigint :amount_paise, null: false
      t.timestamps
    end

    add_check_constraint :refunds, "amount_paise >= 0", name: "refunds_amount_non_negative"
  end
end
