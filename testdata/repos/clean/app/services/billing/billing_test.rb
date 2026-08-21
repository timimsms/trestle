# Excluded by `exclude: ["**/*_test.rb"]`. Never enters the listing at all,
# so no binding can see it and it can never satisfy `discover`.
require "test_helper"

class BillingTest < Minitest::Test
end
