module LegacySearch
  def self.query(term)
    Elasticsearch.search(term)
  end
end
