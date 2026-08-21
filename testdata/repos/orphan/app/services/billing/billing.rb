module Billing
  class Service
    def charge(invoice)
      InvoiceBuilder.new(invoice).build
    end
  end
end
