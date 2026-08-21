# Polls for SLA breaches and escalates. Bound to node `job_sla_monitor`.
module Sla
  class SlaMonitorJob
    def perform
      WorkOrder.breaching_sla.each { |wo| Notifications::Notifier.new.escalate(wo) }
    end
  end
end
