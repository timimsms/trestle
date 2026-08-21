# Creates and transitions work orders. Bound to node `svc_work_orders`.
module WorkOrders
  class WorkOrderService
    def create(tenant:, description:)
      WorkOrder.create!(tenant: tenant, description: description, state: :submitted)
    end
  end
end
