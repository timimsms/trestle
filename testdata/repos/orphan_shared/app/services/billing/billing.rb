module Billing
  class Service
    def charge(invoice)
      Logging.info("charging #{invoice.id}")
    end
  end
end
