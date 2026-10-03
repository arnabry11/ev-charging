class ServiceResponse
  def self.success(payload, status: :ok)
    new(payload:, status:)
  end

  def self.error(error, status: :unprocessable_content)
    new(payload: { error: }, status:)
  end

  def initialize(payload:, status:)
    @payload = payload
    @status = status
  end

  attr_reader :payload, :status
end
