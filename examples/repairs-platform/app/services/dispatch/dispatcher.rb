# Selects a vendor and dispatches the job. Bound to node `svc_dispatch`.
module Dispatch
  class Dispatcher
    def call(work_order)
      VendorRegistry::Registry.new.best_for(work_order)
    end
  end
end
