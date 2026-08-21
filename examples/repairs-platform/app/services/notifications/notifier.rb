# Outbound SMS and email. Bound to node `svc_notifications`.
module Notifications
  class Notifier
    def sms(to:, body:)
      HttpClient::Client.new(base_url: TWILIO_URL).post("/messages", to: to, body: body)
    end
  end
end
