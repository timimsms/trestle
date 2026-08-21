# Declared in `shared:` — real code, deliberately owned by no node.
module HttpClient
  class Client
    def initialize(base_url:) = @base_url = base_url
    def post(path, **body) = Net::HTTP.post(URI.join(@base_url, path), body.to_json)
  end
end
