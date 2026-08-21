module Billing
  class PaymentProcessor
    def capture(invoice)
      StripeGateway::Client.new.charge(invoice.cents)
    end
  end
end
