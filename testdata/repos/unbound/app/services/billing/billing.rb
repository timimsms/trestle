module Billing
  class Service
    def charge(invoice)
      DispatchQueue.push(invoice.id)
    end
  end
end
