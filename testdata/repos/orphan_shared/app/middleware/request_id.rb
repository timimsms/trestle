class RequestId
  def call(env)
    env["request_id"] ||= SecureRandom.uuid
  end
end
