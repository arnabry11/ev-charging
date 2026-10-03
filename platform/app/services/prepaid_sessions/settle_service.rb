module PrepaidSessions
  class SettleService
    def initialize(session_id:, gst_rate_percent: ENV.fetch("GST_RATE_PERCENT", 18).to_i)
      @session_id = session_id
      @gst_rate_percent = gst_rate_percent
    end

    def call
      session = PrepaidSession.find_by(id: session_id)
      return ServiceResponse.error("session_not_stopped") unless session&.state == "stopped"
      return ServiceResponse.success({ invoice: session.invoice.as_registry_json }) if session.invoice
      return ServiceResponse.error("meters_missing") if session.meter_start_wh.nil? || session.meter_stop_wh.nil?
      return ServiceResponse.error("invalid_gst_rate") unless gst_rate_percent.between?(0, 100)

      priced = Billing::Price.calculate(
        meter_start_wh: session.meter_start_wh,
        meter_stop_wh: session.meter_stop_wh,
        energy_price_paise: session.energy_price_paise,
        session_fee_paise: session.session_fee_paise,
        prepaid_paise: session.prepaid_paise,
        gst_rate_percent:
      )

      invoice = create_settlement(session, priced)
      ServiceResponse.success({ invoice: invoice.as_registry_json }, status: :created)
    rescue ActiveRecord::RecordNotUnique
      ServiceResponse.success({ invoice: PrepaidSession.find(session_id).invoice.as_registry_json })
    end

    private

    attr_reader :session_id, :gst_rate_percent

    def create_settlement(session, priced)
      Invoice.transaction do
        invoice = session.create_invoice!(
          tenant: session.tenant,
          energy_wh: priced[:energy_wh],
          taxable_paise: priced[:taxable_paise],
          gst_rate_percent: priced[:gst_rate_percent],
          cgst_paise: priced[:cgst_paise],
          sgst_paise: priced[:sgst_paise],
          total_paise: priced[:total_paise]
        )
        session.create_refund!(tenant: session.tenant, amount_paise: priced[:refund_paise])
        invoice
      end
    end
  end
end
