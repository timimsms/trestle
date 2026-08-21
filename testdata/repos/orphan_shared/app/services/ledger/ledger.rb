module Ledger
  class Service
    def post(entry)
      Entry.create!(entry)
    end
  end
end
