# Trestle — worked example: Repairs & Maintenance platform

This is a **live test input**, not documentation.

- Gate B verifies node extraction against `system.d2` — it must yield 12 IDs with
  container paths intact (`platform.svc_work_orders`).
- Phase 2 asserts `directive` parses 6 binds, 2 external, 2 infra, 1 ignore from it.
- `.trestle.yml` is the config round-trip fixture.
- Phase 4 runs `trestle check` here end to end.

## The code tree is real (Phase 4)

The `discover:` and `shared:` rules used to point at directories that did not exist,
so `trestle check` here was a documented no-op. Phase 4 resolved that the way the
phase file's first option says: **the example now has a small real tree**, consistent
with its own bindings. The config was not weakened — `discover:`, `shared:` and
`exclude:` are untouched.

```
app/services/work_orders/    @bind svc_work_orders
app/services/dispatch/       @bind svc_dispatch
app/services/vendor_registry/@bind svc_vendor_registry
app/services/notifications/  @bind svc_notifications
app/models/work_order.rb     @bind svc_work_orders  (second glob — one node, two globs)
app/jobs/sla/                @bind job_sla_monitor
app/middleware/              shared:
lib/http_client/             shared:
lib/logging/                 shared:
```

There is deliberately **no** `app/services/legacy_tickets/`: `svc_legacy_tickets` is
`@ignore`d with a reason, so it is a node with no code, and creating a directory for it
would make `discover:` demand an owner the diagram has said it does not have.

`app/services/work_orders/work_order_service_spec.rb` exists to exercise
`exclude: ["**/*_spec.rb"]`. Its directory holds a non-spec file too, so excluding it
does not empty the unit.

## Expected result

```console
$ trestle check
0 failures, 1 warnings          # exit 0
$ trestle check --strict
                                # exit 1
```

The one warning is `UNBOUND tenant`, and it is correct rather than something to
suppress. `tenant` is a leaf with no directive — a genuine modeling gap the diagram
has not resolved. The `platform` container does **not** warn, because every one of its
descendants is accounted for (O9). Those two facts together are the O8/O9 resolutions
made visible, and `internal/integration` asserts them.

## Two copies of `system.d2`

`system.d2` and `docs/architecture/system.d2` are byte-identical, and a test in
`cmd/trestle` fails if they ever stop being. The duplication is not laziness:

- `examples/repairs-platform/system.d2` is the canonical Gate B / Phase 2 test input,
  named by OVERVIEW, HANDOFF, PHASE_0 and four `_test.go` files.
- `.trestle.yml` declares `diagrams: [docs/architecture/*.d2]`, and Phase 2's config
  round-trip test asserts that exact value — so the diagram has to be findable there
  for `trestle check` to have anything to check.

Neither could move without editing a Phase 1–3 test. Edit `system.d2` and copy it to
`docs/architecture/system.d2`; the test will tell you if you forget.
