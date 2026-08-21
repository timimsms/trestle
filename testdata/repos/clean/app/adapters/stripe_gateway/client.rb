module StripeGateway
  class Client
    def charge(cents)
      HttpClient.post("/charges", amount: cents)
    end
  end
end
