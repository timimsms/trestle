# Bound to node `svc_work_orders` by the second @bind glob,
# `app/models/work_order*.rb` — one node, two globs, ORed.
class WorkOrder < ApplicationRecord
  belongs_to :tenant
  enum state: { submitted: 0, dispatched: 1, completed: 2 }
end
