module Internal
  module V1
    class InvoicesController < ApplicationController
      skip_forgery_protection
      layout false

      def show
        @session = Tenant.poc.prepaid_sessions.find(params[:id])
        @invoice = @session.invoice
        @refund = @session.refund
        if @invoice.nil? || @refund.nil?
          head :not_found
        else
          render :show
        end
      end
    end
  end
end
