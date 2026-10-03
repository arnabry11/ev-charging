class SettleSessionJob
  include Sidekiq::Job

  sidekiq_options queue: "default", retry: 5

  def perform(session_id)
    PrepaidSessions::SettleService.new(session_id:).call
  end
end
