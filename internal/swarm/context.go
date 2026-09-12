package swarm

import (
	"fmt"
	"sort"
	"strings"
)

// Per-turn context assembly for the "cut tokens per honeybee" lever (ROI
// Priority 1 — Working set / retrieval: "cache/pin file state so unchanged files
// aren't re-read every turn; feed diffs, not full re-reads"; History: "rolling
// summary of prior turns instead of re-injecting the full transcript").
//
// The naive turn loop sends a bare "continue" and leans entirely on the agent to
// re-orient — re-reading the same files in full and re-scanning the whole prior
// transcript every turn. Those re-read tool calls are the dominant per-turn
// GROWTH of the session, and the session is re-sent to the model on every turn,
// so the cost compounds. turnCompactor bounds what the runner hands the agent each turn
// to (changed-file diffs + a rolling summary of prior turns) so the agent
// re-orients WITHOUT re-reading unchanged files. It is a compression of the
// working context, not a drop of the record: the full transcript remains the
// authoritative history on the session branch. Ships inert (LeanContext off) —
// see Runner.leanContextPrompt.

// defaultSummaryCap bounds the rolling transcript digest (bytes). A turn's
// injected history is capped here; earlier scrollback is elided (with a marker),
// never the most recent tail the turn needs to continue.
const defaultSummaryCap = 4096

// turnCompactor carries, across ONE honeybee's turns, the state needed to bound each
// turn's re-sent context. pins is the content the runner last surfaced for each
// file the agent has touched; a turn whose content is unchanged since its pin is
// reported as a one-line no-op rather than re-sent in full, and a changed file is
// reported as a unified diff. summaryCap bounds the rolling transcript digest.
// A nil *turnCompactor is never used (the default path skips assembly entirely).
type turnCompactor struct {
	pins       map[string]string // path -> content last surfaced to the agent
	summaryCap int               // rolling-digest byte cap (0 -> defaultSummaryCap)

	// decisions is the DISTILLED decision log accumulated across this honeybee's
	// turns (analysis C). The former rollingSummary byte-sliced the transcript
	// (head+tail, middle elided on a char boundary), so a decision the agent made
	// mid-session silently vanished once it scrolled out of the tail — the agent then
	// re-derived or CONTRADICTED its earlier choice. Instead we extract the salient
	// lines (choices, root causes, fixes, test/verify results) each turn and PIN them
	// here, so a decision survives for the whole session regardless of scrollback.
	decisions    []string
	seenDecision map[string]bool
}

func newTurnCompactor() *turnCompactor {
	return &turnCompactor{pins: map[string]string{}, seenDecision: map[string]bool{}}
}

// assemble builds the bounded per-turn context to inject next: the changed-file
// diffs (fileFeed), the rolling transcript summary, and the caller's continue
// instruction. This is the runner-authored "opencode session feed" under
// LeanContext, replacing the naive re-inject-everything payload (reinjectAll).
// It advances the file pins (via fileFeed) so the NEXT turn diffs against what
// the agent has now.
func (tc *turnCompactor) assemble(instruction, transcript string, current map[string]string) string {
	feed := tc.fileFeed(current)
	summary := tc.rollingSummary(transcript)
	var b strings.Builder
	b.WriteString("# Bounded turn context (diffs + rolling summary; full history is on the session branch)\n")
	b.WriteString("You already have the files you read/wrote in earlier turns. Only what changed since your " +
		"last turn is shown below — do NOT re-read unchanged files; rely on these diffs and the summary.\n\n")
	if strings.TrimSpace(feed) != "" {
		b.WriteString("## Working-set changes since last turn\n")
		b.WriteString(feed)
		if !strings.HasSuffix(feed, "\n") {
			b.WriteByte('\n')
		}
		b.WriteByte('\n')
	}
	if strings.TrimSpace(summary) != "" {
		b.WriteString("## Summary of prior turns\n")
		b.WriteString(summary)
		b.WriteString("\n\n")
	}
	b.WriteString(instruction)
	return b.String()
}

// fileFeed returns the bounded per-turn file payload for current (path -> the
// latest content the agent has). A path unchanged since it was last surfaced
// yields a single "unchanged" no-op line — NOT the full file; a changed path
// yields a unified diff; a newly-seen path is pinned silently (the agent already
// has it from its own read, so re-emitting the full file is exactly the waste we
// cut). Pins advance to current so the next turn diffs against what the agent has
// now. Deterministic: paths are sorted.
func (tc *turnCompactor) fileFeed(current map[string]string) string {
	if tc.pins == nil {
		tc.pins = map[string]string{}
	}
	paths := make([]string, 0, len(current))
	for p := range current {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	var diffs []string
	var unchanged []string
	for _, p := range paths {
		cur := current[p]
		prev, seen := tc.pins[p]
		tc.pins[p] = cur
		switch {
		case !seen:
			// First time surfaced: pin silently, do not re-emit the full file.
		case prev == cur:
			unchanged = append(unchanged, p)
		default:
			diffs = append(diffs, p+":\n"+lineDiff(prev, cur))
		}
	}

	var b strings.Builder
	for _, d := range diffs {
		b.WriteString(d)
		if !strings.HasSuffix(d, "\n") {
			b.WriteByte('\n')
		}
	}
	if len(unchanged) > 0 {
		fmt.Fprintf(&b, "unchanged since last turn: %s\n", strings.Join(unchanged, ", "))
	}
	return b.String()
}

// rollingSummary folds the full transcript into a bounded digest. Under the cap it
// is returned verbatim; over the cap the head (early task framing / decisions) and
// the tail (most recent state / outputs) are kept and the middle scrollback is
// elided with a marker. The tail is NEVER dropped, so the state a turn needs to
// continue and to satisfy the completion check survives the compression.
func (tc *turnCompactor) rollingSummary(transcript string) string {
	cap := tc.summaryCap
	if cap <= 0 {
		cap = defaultSummaryCap
	}
	// Distill this turn's salient decisions and accumulate them (dedup) so a choice
	// made early survives the whole session, then present the pinned decision log
	// PLUS the most recent tail (immediate continuity). This replaces the pure
	// byte-slice, which dropped mid-session reasoning and let the agent contradict
	// its own earlier decisions.
	for _, d := range distillDecisions(transcript) {
		if tc.seenDecision == nil {
			tc.seenDecision = map[string]bool{}
		}
		if tc.seenDecision[d] {
			continue
		}
		tc.seenDecision[d] = true
		tc.decisions = append(tc.decisions, d)
	}

	t := strings.TrimRight(transcript, "\n")

	// Budget: reserve up to half the cap for the accumulated decision log (newest
	// wins if it overflows), the rest for the recent tail.
	var log string
	if len(tc.decisions) > 0 {
		logCap := cap / 2
		prefix := "Decisions & findings so far (pinned across turns — do not re-derive or contradict these):\n"
		// Keep the most RECENT decisions when the log overflows its budget.
		start := 0
		for {
			var probe strings.Builder
			probe.WriteString(prefix)
			for _, d := range tc.decisions[start:] {
				probe.WriteString("- " + d + "\n")
			}
			if probe.Len() <= logCap || start >= len(tc.decisions)-1 {
				log = probe.String()
				break
			}
			start++
		}
	}

	tailCap := cap - len(log)
	if tailCap < 0 {
		tailCap = 0
	}
	tail := t
	truncated := false
	if len(tail) > tailCap {
		truncated = true
		tail = tail[len(tail)-tailCap:]
		if i := strings.IndexByte(tail, '\n'); i >= 0 && i+1 < len(tail) {
			tail = tail[i+1:] // start on a line boundary
		}
	}

	// No decisions and nothing elided: return the tail verbatim (unchanged from the
	// historical under-cap behavior). Only add framing when there is something to
	// frame — a pinned decision log or an honest elision marker.
	if log == "" && !truncated {
		return tail
	}
	var out strings.Builder
	if log != "" {
		out.WriteString(log)
		out.WriteByte('\n')
	}
	if truncated {
		out.WriteString("… [earlier scrollback elided; full transcript on the session branch] …\n")
	}
	if log != "" {
		out.WriteString("Most recent activity:\n")
	}
	out.WriteString(tail)
	return strings.TrimRight(out.String(), "\n")
}

// decisionCues mark a transcript line as a durable decision/finding worth pinning
// across turns: a design choice, a root cause, a fix, or a test/verification
// result. Matched case-insensitively as substrings.
var decisionCues = []string{
	"decid", "chose", "choose", "chosen", "because", "instead", "approach",
	"root cause", "turns out", "tradeoff", "trade-off", "rejected", "the reason",
	"fixed", "the fix", "the bug", "regression test", "passes", "passed", "failing",
	"verified", "confirmed", "note that", "gotcha", "blocked on", "depends on",
}

// distillDecisions extracts the salient decision/finding lines from a transcript.
// It is a cheap, deterministic heuristic (no model call): it keeps lines carrying
// a decisionCue, normalizes whitespace, trims each to a sane length, and skips
// noise (diff/tool-output lines, headings). Order-preserving; dedup is the
// caller's (it accumulates across turns).
func distillDecisions(transcript string) []string {
	var out []string
	for _, raw := range strings.Split(transcript, "\n") {
		ln := strings.TrimSpace(raw)
		if ln == "" {
			continue
		}
		// Skip obvious non-prose noise: diff/patch lines and tool-output framing.
		if strings.HasPrefix(ln, "+") || strings.HasPrefix(ln, "-") ||
			strings.HasPrefix(ln, "@@") || strings.HasPrefix(ln, "#") {
			continue
		}
		low := strings.ToLower(ln)
		hit := false
		for _, c := range decisionCues {
			if strings.Contains(low, c) {
				hit = true
				break
			}
		}
		if !hit {
			continue
		}
		ln = strings.Join(strings.Fields(ln), " ") // normalize internal whitespace
		const maxLine = 240
		if len(ln) > maxLine {
			ln = ln[:maxLine] + "…"
		}
		out = append(out, ln)
	}
	return out
}

// reinjectAll is the re-inject-everything baseline the diff feed replaces: the
// entire verbatim transcript plus every surfaced file in full. It quantifies the
// per-turn byte cut (tests compare assemble() against it) and documents precisely
// what LeanContext avoids sending each turn. Pure: it never touches pins.
func reinjectAll(transcript string, current map[string]string) string {
	paths := make([]string, 0, len(current))
	for p := range current {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	var b strings.Builder
	b.WriteString(strings.TrimRight(transcript, "\n"))
	b.WriteByte('\n')
	for _, p := range paths {
		fmt.Fprintf(&b, "%s:\n%s\n", p, current[p])
	}
	return b.String()
}

// latestFileContents extracts, from a session transcript, the most recent full
// content the agent has for each file it read, wrote, or EDITED. A completed
// read's output and a write's input content are full snapshots taken directly.
// An edit carries only an (oldString -> newString) fragment, not the whole file,
// so a COMPLETED edit is APPLIED (applyEdit) to the running snapshot the map
// already holds for that path — edits are the dominant way a honeybee mutates a
// file across turns, so ignoring them left a just-edited file mis-reported as
// "unchanged" by the diff feed (the file changed but no read/write re-snapshotted
// it). Parts are processed in order so sequential edits compose and a later
// read/write overrides. An edit to a path with no prior snapshot is skipped: a
// fragment alone cannot reconstruct the whole file. (The multi-hunk `patch` tool
// is still not folded — its unified-diff payload needs a full applier; a
// subsequent read re-snapshots such files.)
func latestFileContents(msgs []Message) map[string]string {
	out := map[string]string{}
	for _, m := range msgs {
		for _, p := range m.Parts {
			if p.Type != "tool" {
				continue
			}
			path := inputString(p.Input, "filePath")
			if path == "" {
				continue
			}
			switch p.Tool {
			case "read":
				if p.Status == "completed" && p.Output != "" {
					out[path] = p.Output
				}
			case "write":
				if c := inputString(p.Input, "content"); c != "" {
					out[path] = c
				}
			case "edit":
				// Only a completed edit actually mutated the file; a
				// pending/errored one (e.g. oldString absent or ambiguous)
				// left it unchanged. Need a prior full snapshot to apply to.
				if p.Status != "completed" {
					continue
				}
				if base, ok := out[path]; ok {
					out[path] = applyEdit(base, p.Input)
				}
			}
		}
	}
	return out
}

// applyEdit folds a completed edit tool's (oldString -> newString) fragment into
// the content the runner already tracks for that file, mirroring the editor's own
// single-occurrence (or replaceAll) substitution so the diff feed reflects an
// in-place edit instead of reporting the file unchanged. Best-effort: an empty or
// absent oldString, or one not present in base (a stale snapshot), leaves base
// untouched — the runner never re-reads to reconcile and the authoritative file
// still streams verbatim to the session branch. replaceAll replaces every
// occurrence; otherwise only the first (the single unique match a plain edit made).
func applyEdit(base string, in map[string]any) string {
	oldS := inputString(in, "oldString")
	if oldS == "" || !strings.Contains(base, oldS) {
		return base
	}
	newS := inputString(in, "newString")
	if all, ok := in["replaceAll"].(bool); ok && all {
		return strings.ReplaceAll(base, oldS, newS)
	}
	return strings.Replace(base, oldS, newS, 1)
}

// transcriptText renders the transcript to plain text (assistant/user text,
// reasoning, and tool commands + output) for the rolling digest. It is the raw
// content the recorder renders to markdown, without the markdown chrome, so the
// digest measures real conversational bytes.
func transcriptText(msgs []Message) string {
	var b strings.Builder
	for _, m := range msgs {
		for _, p := range m.Parts {
			switch p.Type {
			case "text", "reasoning":
				if s := strings.TrimSpace(p.Text); s != "" {
					b.WriteString(m.Role + ": " + s + "\n")
				}
			case "tool":
				fmt.Fprintf(&b, "tool %s %s\n", p.Tool, inputSummary(p.Tool, p.Input))
				switch {
				case p.Status == "error" && strings.TrimSpace(p.Error) != "":
					b.WriteString(p.Error + "\n")
				case strings.TrimSpace(p.Output) != "":
					b.WriteString(p.Output + "\n")
				}
			}
		}
	}
	return b.String()
}

// lineDiff renders a minimal unified-style line diff of old -> new: the common
// leading and trailing lines are trimmed and only the changed hunk is emitted, so
// a small edit in a large file yields a small payload instead of the whole file.
// old==new is never passed here (fileFeed reports that as a no-op). Deterministic.
func lineDiff(oldS, newS string) string {
	o := strings.Split(oldS, "\n")
	n := strings.Split(newS, "\n")
	// Common prefix.
	p := 0
	for p < len(o) && p < len(n) && o[p] == n[p] {
		p++
	}
	// Common suffix (not overlapping the prefix).
	s := 0
	for s < len(o)-p && s < len(n)-p && o[len(o)-1-s] == n[len(n)-1-s] {
		s++
	}
	var b strings.Builder
	fmt.Fprintf(&b, "@@ -%d,%d +%d,%d @@\n", p+1, len(o)-p-s, p+1, len(n)-p-s)
	for _, l := range o[p : len(o)-s] {
		b.WriteString("-" + l + "\n")
	}
	for _, l := range n[p : len(n)-s] {
		b.WriteString("+" + l + "\n")
	}
	return b.String()
}

// inputString reads a string field from a tool part's input arguments, tolerating
// a non-string value by formatting it. Missing -> "".
func inputString(in map[string]any, key string) string {
	v, ok := in[key]
	if !ok || v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprint(v)
}
