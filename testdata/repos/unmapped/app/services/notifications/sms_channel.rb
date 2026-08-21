module Notifications
  class SmsChannel
    def deliver(customer)
      Twilio.send(customer.phone)
    end
  end
end
