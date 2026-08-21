module HttpClient
  def self.post(path, **params)
    Net::HTTP.post(path, params)
  end
end
