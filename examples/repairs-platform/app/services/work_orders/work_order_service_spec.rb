# Excluded by `exclude: ["**/*_spec.rb"]` — a test is not architecturally real.
require "spec_helper"

RSpec.describe WorkOrders::WorkOrderService do
  it "creates a work order" do
    expect(described_class.new).to respond_to(:create)
  end
end
