module BillingSearch
  class Index
    def query(term)
      LegacySearch.query(term)
    end
  end
end
