module Search
  class Service
    def query(term)
      Indexer.new.lookup(term)
    end
  end
end
