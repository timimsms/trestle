class Reconciler
  def perform
    Ledger::Service.new.reconcile
  end
end
