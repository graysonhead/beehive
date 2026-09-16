# Skill: writing a definition-of-done check that catches integration failures

Read this when AUTHORING a task's `Check:` (a bootstrap/reconcile pass) or JUDGING one
(a review pass). It complements — does not replace — `docs/checks-framework-registry.md`
and the reconcile/bootstrap "give every task a definition of done" rule. Those already
force every `Check:` to invoke a REAL approved framework and refuse bare source-greps.
This skill covers the failure that survives that rule: **a real-framework check that is
too NARROW to prove the task's actual intent.**

## The failure this exists to stop

A framework check can be genuine and still prove almost nothing. Concrete corpus example:
a task "migrate the session store onto the shared keyed store" shipped with
`Check: cargo test -p pillar-net --lib kv_plane_migration` — a real `cargo test`, passing,
CHECKS.md-approved. It proved the crate *compiles and its own unit swapped a map for a
store*. It did NOT prove the thing the ROI asked for: that the LIVE system now folds that
plane through the ONE shared substrate and surfaces it as an inspectable collection. Three
disconnected store instances shipped, every migration marked DONE, the user-facing surface
empty. Every check was "real"; every check was too narrow. That is the disease: **a check
that asserts the code was written, not that the effect the operator wanted is true end to
end.**

## The test for a good DoD check

Before accepting a `Check:`, ask: *if the implementer built a parallel, disconnected,
locally-correct version that does not actually wire into the live system, would this check
still pass?* If yes, the check is too narrow — tighten it.

A good check asserts the **observable effect at the seam the intent cares about**, not the
existence of a symbol or the greenness of one crate's unit test:

- Prefer the **highest-level assertion that still runs deterministically**: an
  integration/e2e test that exercises the real path over a unit test of one function; a
  live probe (`curl … | grep`, `kubectl rollout status`, pull-by-digest,
  `beehive audit`) over an in-crate assertion; a query against the running surface over a
  construction-site test.
- For a **migration / "route X through the shared Y"** task, the check must assert X's data
  is observable **through Y's live surface** (the shared instance, the portal/CLI query,
  the catalog) — not that a `Y::new()` appears in X's crate. "Old path removed" is part of
  done: assert the bespoke store is GONE (no second instance constructed), not merely that
  a new one exists alongside it.
- For a **cross-cutting invariant** (one substrate, one source of truth, a global
  property), the check must exercise that invariant across the seam — see the acceptance
  task below.

## Cross-cutting intent needs an acceptance task, not just per-leaf checks

Per-leaf checks cannot see the seam between leaves; integration is the gap between tasks,
and no leaf owns a gap. When an ROI item implies a property that spans several tasks (a
shared substrate, an end-to-end user-visible surface, a global invariant), emit — in
addition to the leaf tasks — a **terminal acceptance task** that:

- `deps=` **every leaf** in the cluster (it runs only once they are all DONE), and inherits
  the **cluster's top ROI tier weight** (never a low tier — a leaf-dependent successor
  otherwise starves in the lottery);
- carries an **`Invariant:`** line naming the cross-cutting property in one sentence (the
  thing review must verify, e.g. "all internal planes fold through the one shared cell
  keyed-store and appear in `pillar catalog collections` on a live cell");
- has a **`Check:`** that exercises that invariant against the RUNNING system (a live query
  / integration probe), matching an approved CHECKS.md framework.

The acceptance task is where "does the feature actually work, integrated" becomes some
agent's owned, gated deliverable instead of nobody's.

## The `Invariant:` task-body field

Any task that carries or must preserve a cross-cutting contract gets an `Invariant:` line
in its body (freeform body text, round-tripped verbatim; not a header tag). It is the
reviewer's INPUT: review judges the work against `Invariant:` + `Context:` + the live
`Check:`, not against the diff in isolation. Absent an `Invariant:`, review falls back to
the task's `Context:` and ROI intent.

## For the review pass

- RUN the `Check:` (`beehive task check <sm> <id>`) and confirm it passes AND is not the
  narrow kind above. A passing-but-narrow check on a task with a cross-cutting `Invariant:`
  is a rejection just like a failing check.
- If the work is locally sound but the check is too narrow to prove the `Invariant:`, do not
  approve on the weak check — send it back with `beehive task reject … --feedback` naming
  the missing assertion, or (if the gap is a genuinely separate deliverable) file the
  acceptance/integration task with `beehive task add … --check '<live probe>'`.
