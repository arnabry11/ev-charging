module Internal
  module V1
    class GatewayEventsController < ApplicationController
      skip_forgery_protection

      def create
        unless signature.valid?
          render json: { error: "invalid_signature" }, status: :unauthorized
          return
        end

        result = GatewayEvents::ReceiveService.new(payload: JSON.parse(request.raw_post)).call
        render json: result.payload, status: result.status
      rescue JSON::ParserError
        render json: { error: "invalid_event" }, status: :unprocessable_content
      end

      private

      def signature
        GatewayEvents::Signature.new(
          secret: ENV.fetch("GATEWAY_SIGNING_SECRET"),
          timestamp: request.headers["X-Timestamp"],
          signature: request.headers["X-Signature"],
          body: request.raw_post
        )
      end
    end
  end
end
