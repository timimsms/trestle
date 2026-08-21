# The vendor catalogue. Bound to node `svc_vendor_registry`.
module VendorRegistry
  class Registry
    def best_for(work_order)
      vendors.min_by { |v| v.distance_to(work_order.site) }
    end
  end
end
