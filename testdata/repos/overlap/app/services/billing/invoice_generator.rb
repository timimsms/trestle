module Billing
  class InvoiceGenerator
    def generate(order)
      Invoice.create!(order: order)
    end
  end
end
