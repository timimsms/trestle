module Dispatch
  class Dispatcher
    def enqueue(work_order_id)
      Lula::Client.new.dispatch(work_order_id)
    end
  end
end
