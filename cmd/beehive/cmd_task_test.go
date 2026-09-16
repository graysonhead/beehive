package main

import (
	"testing"
	"time"

	"github.com/spencerharmon/beehive/internal/plan"
)

// TestHumanReason proves humanReason PRESERVES a --reason's embedded line
// structure (needs-human-standard-escalation-format): the standard action-first
// escalation template (summary, then Steps/Links/Technical detail each on their
// own markdown line) is authored with real newlines, so a multi-line --reason
// must survive as multiple lines — only each line's OWN internal whitespace
// (stray tabs/repeated spaces) is normalized — rather than the whole reason
// being collapsed into one unreadable run-on sentence.
func TestHumanReason(t *testing.T) {
	got, err := humanReason(" Need\noperator\tinput ", "")
	if err != nil {
		t.Fatal(err)
	}
	if got != "Need\noperator input" {
		t.Fatalf("reason = %q", got)
	}
	multi, err := humanReason("Summary line.\n1. Do the first step.\n2. Do the second step.\nLinks: https://example.com/dash", "")
	if err != nil {
		t.Fatal(err)
	}
	if multi != "Summary line.\n1. Do the first step.\n2. Do the second step.\nLinks: https://example.com/dash" {
		t.Fatalf("multi-line reason not preserved: %q", multi)
	}
	if _, err := humanReason("", ""); err == nil {
		t.Fatal("empty reason allowed")
	}
	if _, err := humanReason("x", "y"); err == nil {
		t.Fatal("reason and reason-file both allowed")
	}
}

func TestHumanCategory(t *testing.T) {
	for _, in := range []string{"secret", "external-permission", "contradiction", "architecture", " secret "} {
		c, err := humanCategory(in)
		if err != nil {
			t.Fatalf("%q: %v", in, err)
		}
		if !c.Valid() {
			t.Fatalf("%q -> invalid category %q", in, c)
		}
	}
	for _, in := range []string{"", "  ", "maintenance", "cache-clear", "SECRET"} {
		if _, err := humanCategory(in); err == nil {
			t.Fatalf("category %q accepted", in)
		}
	}
	if got, _ := humanCategory("contradiction"); got != plan.CatContradiction {
		t.Fatalf("category = %q, want %q", got, plan.CatContradiction)
	}
}

func TestTaskSubmoduleName(t *testing.T) {
	for in, want := range map[string]string{
		"alpha":            "alpha",
		"submodules/alpha": "alpha",
		"alpha/":           "alpha",
	} {
		got, err := taskSubmoduleName(in)
		if err != nil {
			t.Fatalf("%s: %v", in, err)
		}
		if got != want {
			t.Fatalf("%s -> %q, want %q", in, got, want)
		}
	}
	for _, in := range []string{".", "..", "submodules", "../x", "alpha/beta"} {
		if _, err := taskSubmoduleName(in); err == nil {
			t.Fatalf("%s accepted", in)
		}
	}
}

// TestSetTaskFeedback proves the review FEEDBACK disposition records a single,
// current `Feedback:` line in the task body and SUPERSEDES any prior one, so a
// reworked task never accumulates stale reviewer guidance across rounds.
func TestSetTaskFeedback(t *testing.T) {
	task := &plan.Task{Body: []string{"Context: do the thing", ""}}

	setTaskFeedback(task, "  close the   integration gap: assert the live surface  ")
	got := feedbackLines(task)
	if len(got) != 1 {
		t.Fatalf("want exactly one Feedback: line, got %d: %q", len(got), task.Body)
	}
	if got[0] != "Feedback: close the integration gap: assert the live surface" {
		t.Fatalf("feedback not normalized/recorded: %q", got[0])
	}

	// A second round supersedes, never appends a duplicate.
	setTaskFeedback(task, "still missing the tombstone case")
	got = feedbackLines(task)
	if len(got) != 1 {
		t.Fatalf("second feedback must supersede, got %d Feedback: lines: %q", len(got), task.Body)
	}
	if got[0] != "Feedback: still missing the tombstone case" {
		t.Fatalf("superseded feedback wrong: %q", got[0])
	}
	// The original Context: line survives.
	if task.Body[0] != "Context: do the thing" {
		t.Fatalf("body prelude clobbered: %q", task.Body)
	}
}

// TestRejectFromReviewLoopCap proves the disposition's state semantics: a reject
// from NEEDS-REVIEW returns the task to TODO and bumps attempts, and once attempts
// exceed reject_limit it escalates to NEEDS-HUMAN rather than looping forever.
func TestRejectFromReviewLoopCap(t *testing.T) {
	now := time.Now().UTC()
	task := &plan.Task{ID: "x", Status: plan.StatusReview}
	const limit = 3
	for i := 1; i <= limit; i++ {
		task.Status = plan.StatusReview // a fresh review each round precedes the reject
		if err := task.Reject(limit, now); err != nil {
			t.Fatalf("round %d reject: %v", i, err)
		}
		if task.Status != plan.StatusTODO {
			t.Fatalf("round %d: want TODO, got %s (attempts=%d)", i, task.Status, task.Attempts)
		}
	}
	// One more rejection tips it past the limit -> NEEDS-HUMAN.
	task.Status = plan.StatusReview
	if err := task.Reject(limit, now); err != nil {
		t.Fatalf("overflow reject: %v", err)
	}
	if task.Status != plan.StatusHuman {
		t.Fatalf("past reject_limit=%d (attempts=%d) want NEEDS-HUMAN, got %s", limit, task.Attempts, task.Status)
	}
}

func feedbackLines(t *plan.Task) []string {
	var out []string
	for _, l := range t.Body {
		if len(l) >= len("Feedback:") && l[:len("Feedback:")] == "Feedback:" {
			out = append(out, l)
		}
	}
	return out
}
