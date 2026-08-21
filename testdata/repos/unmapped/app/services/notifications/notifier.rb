module Notifications
  class Notifier
    def receipt(invoice)
      SmsChannel.new.deliver(invoice.customer)
    end
  end
end
