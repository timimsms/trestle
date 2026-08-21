# Declared in `shared:` — real code, deliberately owned by no node.
module Logging
  def self.logger = @logger ||= Logger.new($stdout)
end
