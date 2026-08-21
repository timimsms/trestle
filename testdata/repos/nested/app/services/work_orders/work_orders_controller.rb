module WorkOrders
  class WorkOrdersController < ApplicationController
    def create
      Dispatch::Dispatcher.new.enqueue(params[:id])
    end
  end
end
