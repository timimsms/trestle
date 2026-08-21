# Declared in `shared:` — real code, deliberately owned by no node.
class RequestId
  def initialize(app) = @app = app
  def call(env) = @app.call(env.merge("request_id" => SecureRandom.uuid))
end
