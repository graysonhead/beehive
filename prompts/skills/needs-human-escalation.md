# Skill: authoring a NEEDS-HUMAN escalation artifact

Read this ONLY when you are about to file a `beehive task human` whose operator-facing Steps are a
fixed sequence of mechanically-executable commands (host-root ops, a data migration, a cutover, a
multi-step provisioning run). For an ordinary one-line ask (a secret to set, a permission to grant)
you do NOT need this skill — the `--reason` template in `HONEYBEE.md` (Steps §4) is enough.

The rule this skill governs is the SHAPE of what you hand the operator, not the `--category` gate (still
whichever of the four applies, usually `external-permission`).

## Ship a script, not prose (needs-human-executable-artifact)

If the operator-facing Steps are a fixed sequence of concrete commands, the swarm's job is NOT to
transcribe them into the `--reason` — it is to AUTHOR a single, reviewable, executable artifact (a
`set -euo pipefail` script / a rendered manifest) that the operator reviews once and runs, and reduce
the escalation Steps to "review then run this artifact." The artifact carries the precision the human
cannot be asked to reconstruct: EVERY value the swarm cannot know from inside the sandbox (a host path,
a device, a color, a service name, a credential location) is a DOCUMENTED OPTION/flag with a sane
default, never a blank the operator must infer; every destructive step is gated behind a typed
confirmation and supports a dry run; every phase fails loudly with no silent partial. If the artifact
must exist on `main` before the human step can reference it, SPLIT the work: a normal mergeable
**part (a)** task authors + reviews + merges the script/tooling, and the **part (b)** NEEDS-HUMAN
escalation (hard-dep on part (a) DONE) is only "review then run `path/to/script …`". Vague imperative
prose that could have been a script IS a shortcut — the same class as a placeholder value: it pushes
the swarm's unfinished work onto the operator and invites a mis-execution the script would have
prevented.

## State the EXACT command, not "run the script"

The escalation Steps must give the operator the LITERAL invocation to paste — binary/script path plus
every flag filled with the REAL value this migration needs
(`sudo ./scripts/foo.sh quiesce --gostream-state /var/gostream`), in run order, one line per command.
Never `run the script with the appropriate flags`, never a `<placeholder>` the operator must resolve:
if a value is knowable from the target's state, put it in; if it is genuinely operator-only (a path
only they can confirm), name the ONE thing to fill and where to get it. When the script/test/binary
lives on a worktree branch (a `bee-<taskid>` submodule checkout or a hive `.worktrees/<branch>/`), the
literal invocation is the `beehive [submodule] worktree exec …` form — e.g. `beehive submodule worktree
exec <sm> <branch> -- ./scripts/foo.sh quiesce --gostream-state /var/gostream` — never a `cd <path> &&
…` or a bare relative path.

## ONE command, not a checklist of them

If the artifact has multiple steps, it runs them itself in order from a single entrypoint — do NOT hand
the operator N commands to paste in sequence (that is just prose steps wearing a monospace font). The
one command stops ONLY at the genuine human-decision gates the swarm cannot make (judge private/opaque
data, confirm a user-visible cutover is healthy); everything mechanical between gates is automatic, and
those gates are un-skippable even under a `--yes`/non-interactive flag. Offer the individual steps as an
advanced escape hatch for retry/inspection, but the DEFAULT the escalation names is the single
run-it-all command.

## Bake known values as defaults; surface only what you cannot validate

If you know a value (a path, a namespace, a service name discoverable from the target's state), it is a
DEFAULT inside the artifact — not a flag the operator must supply. The ideal example command is
zero-flag (`sudo ./run.sh migrate`); a flag appears in the example ONLY for a value the operator alone
holds. Validate the baked defaults up front (assert each path/resource exists before mutating) so a
wrong default fails loudly and immediately.

You may only hide a value you have VERIFIED. If the value lives outside your sandbox — a path on the
operator's host, a network resource you cannot reach, an install-specific location — you have NOT
verified it, so guessing a default and hiding it is a placeholder shortcut: a hidden wrong guess is
strictly worse than a flag. Instead: (a) if the artifact can discover the real value at run time on the
target (query the running unit, the API, the cluster), do that and fall back to a flag; (b) else expose
it as a flag — required, or defaulted only to a genuine, documented CONVENTION (a package default),
never to an invented path. A state-file location is whatever the program's own code/config says it is,
not where it "should" live.

## Consume a bulk capture whole

If the operator has already recursively copied a directory tree (a state dir, a data dir), the artifact
copies/mounts that tree verbatim and lets the downstream consumer take what it needs; it must NOT single
out one file by an exact name/path, assert that file, or reconstruct a sub-layout the operator already
provided. Singling out a file is both fragile (an exact path you probably have wrong) and unsafe (you
drop its siblings — e.g. a SQLite `-wal`/`-shm` alongside the `.db`, losing the newest writes). Trust
the operator's completed handoff over your model of its internals.

## Own the whole operation, be safely re-runnable

If the artifact mutates state that a controller continuously reconciles (a GitOps operator, an
autoscaler, an operator/CRD), it must suspend that controller for the duration and restore it after —
discovering the controller's identity from the live objects, not a hardcoded name — or the controller
races the artifact and undoes/corrupts the change mid-flight. On failure, leave the system in the SAFE
half-state (quiesced, not half-started over inconsistent data) and say so, rather than blindly
restoring. Make every step idempotent: a failed run must be fixable and re-run from the top
(delete-and-recreate the one-shot job, skip-if-present, `rsync` not blind copy) without hand-surgery
between attempts.

## Know what is UNDER a directory tree you copy

A source dir can contain nested/virtual mounts (FUSE, bind, network) that are NOT the data you mean to
copy and can be orders of magnitude larger — recursing into them fills the disk with real bytes and
detonates the operation. Copy with the mount-boundary respected (`rsync --one-file-system`, `find
-xdev`, `tar --one-file-system`), and guard capacity up front: measure the REAL source size (`du -sx`,
mounts excluded) against free space on the destination filesystem and refuse before the first byte if it
will not fit. Staging onto the same disk duplicates the data — count that.
