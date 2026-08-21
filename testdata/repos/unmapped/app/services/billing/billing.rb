module Billing
  class Service
    def charge(invoice)
      Notifications::Notifier.new.receipt(invoice)
    end
  end
end
