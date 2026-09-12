# Context-quality levers (analysis A–G)

Why beehive output lagged a one-shot agent session, and what changed. The root causes were context
noise, enforced context starvation, and lossy multi-turn memory. These changes attack all three, mostly
by turning ON and improving machinery that already existed but shipped dormant/env-gated.

## A — Per-kind prompt slicing (default ON)

`trimProtocol` (internal/swarm/inject.go) already trims the injected `HONEYBEE.md` to the sections a
pass's kind acts on (dropping other kinds' role sections + managed boilerplate). It was gated behind an
env-only, default-OFF `BEEHIVE_LEAN_INJECT`. It is now a layered-config lever (`lean_inject`) that
defaults ON (`config.LeanInjectEnabled`). The escalation "executable-artifact" essay (~78 lines that were
resident in the SHARED Steps section on every pass, every kind) was extracted to a lazily-read skill,
`skills/needs-human-escalation.md`; `HONEYBEE.md` dropped 711→639 lines.

## B — Precomputed Work brief (default ON) + package neighborhood

`buildTaskBrief` (internal/swarm/brief.go) was env-gated/off. Now default ON (`lean_brief`). Added:
- **Package neighborhood**: sibling files of the task's `Files:` are surfaced by NAME (not content, name
  cap 24) so the agent reads the local conventions, callers, and tests instead of being blind to what
  surrounds its edit. The former blanket "read only your Files:, not the tree" over-corrected a token
  worry into a quality defect.
- **Change-doc skeleton** (analysis F): the runner authors the deterministic doc STRUCTURE (path,
  `Beehive-Commits` header placeholder, the required evidence headings) so the agent spends turns on the
  real evidence, not reconstructing the doc shape.

## C — Distilled decision log replaces byte-slice compaction (default ON)

`turnCompactor.rollingSummary` (internal/swarm/context.go) byte-sliced the transcript (head+tail, middle
elided on a char boundary), so a decision made mid-session vanished once it scrolled out of the tail and
the agent re-derived or CONTRADICTED it. It now distills salient decision/finding lines each turn
(`distillDecisions`, a cheap deterministic heuristic — no model call) and PINS them across the whole
session, presented above the recent tail. `lean_context` defaults ON. The no-decision path still returns
the tail verbatim under cap / with an honest elision marker over cap.

## D — Coherent vertical slices + `Context:` field (bootstrap/reconcile prompts)

`bootstrap.md` / `reconcile.md` (and the injected `HONEYBEE.md` Reconcile section) now steer task
decomposition toward self-contained vertical slices one isolated agent can hold in full, not the smallest
parallelizable fragment (which loses global coherence and thrashes review). Every new task carries a
`Context: <the why>` line with the parent ROI intent, so the isolated implementer understands its slice's
purpose without opening `ROI.md` (which it may not).

## E — Reviewer recon (review prompts)

The injected `HONEYBEE.md` Review section and `review.md` now require the reviewer to recon broadly — its
worktree is a full read-only checkout, so it must read callers, siblings, and tests, not judge the diff in
isolation. Reviewer model routing is unchanged: `config.ModelFor` falls through to the single strong
`model` for the `review` kind unless an operator explicitly overrides it — keep review on a strong model
(do not cheap-route it in `models:`).

## F — Doc-skeleton (see B). Bookkeeping (worktree/claim/merge/commit-stamp) is already runner-owned.

## G — Prune scar tissue

Extracted the executable-artifact essay to `skills/needs-human-escalation.md` (indexed in `AGENTS.md`) and
condensed the `HONEYBEE.md` reference to a pointer.

## Config

New layered-config keys, each `*bool`, default ON when unset; matching `BEEHIVE_LEAN_*` env vars override
either direction (env wins):

```yaml
lean_inject:  true   # A: trim injected system prompt to the pass kind
lean_brief:   true   # B: precomputed Work brief + neighborhood + doc skeleton
lean_context: true   # C: distilled decision-log turn compaction
```

To restore the historical full-context behavior, set any to `false` (or export `BEEHIVE_LEAN_*=0`).
