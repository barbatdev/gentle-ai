package sddstatus

import (
	"context"
	"fmt"
	"testing"
)

// #3815: RuntimeAttempt was simultaneously one provider call, one unit of
// budget, and one unit of work. A work unit that legitimately needs several
// calls therefore exhausted its objective by ACCOUNTING rather than by
// failure: each begin charged an attempt, so with the default budget of two,
// two calls ended the objective even when both delivered real increment. That
// is #3808, where two calls produced zero delivered production and
// decision_required.
//
// The rule: an interrupted settlement that delivered measurable increment
// advances the objective instead of discharging an attempt against it. A call
// that delivered nothing is still spent, so max_attempts keeps bounding calls
// that produce nothing, and cumulative changed lines keep bounding the total —
// a refund always costs delivered lines, and those are capped.

func beginRuntimeAttempt(t *testing.T, store RuntimeStore, expected, requestID string) RuntimeStatus {
	t.Helper()
	status, err := store.Begin(context.Background(), BeginAttemptRequest{
		ExpectedRevision: expected, RequestID: requestID, WorkUnit: "large-atomic-unit",
		EvidenceGoal: "deliver one atomic cutover across several calls",
		MaxAttempts:  2, MaxChangedLines: 400,
	})
	if err != nil {
		t.Fatalf("Begin(%s): %v", requestID, err)
	}
	return status
}

func interruptRuntimeAttempt(t *testing.T, store RuntimeStore, expected, requestID string) RuntimeStatus {
	t.Helper()
	status, err := store.Finish(context.Background(), FinishAttemptRequest{
		ExpectedRevision: expected, RequestID: requestID, Outcome: AttemptInterrupted,
		Diagnosis:          "the call ended before the atomic unit was complete",
		HarnessDisposition: HarnessReused,
		CleanupEvidence:    "workspace left intact for the successor call",
		ProcessEvidence:    "no descendants remained after the call",
	})
	if err != nil {
		t.Fatalf("Finish(%s): %v", requestID, err)
	}
	return status
}

// TestInterruptedCallThatDeliveredIncrementDoesNotSpendTheObjectiveBudget is
// the #3808 shape: two calls, both delivering, must not exhaust a two-attempt
// objective.
func TestInterruptedCallThatDeliveredIncrementDoesNotSpendTheObjectiveBudget(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	store, err := OpenRuntimeStore(context.Background(), repo, "budget-granularity")
	if err != nil {
		t.Fatal(err)
	}

	status := beginRuntimeAttempt(t, store, "", "granularity-begin-1")
	appendRuntimeLedgerFile(t, repo, "first slice of the cutover\n")
	status = interruptRuntimeAttempt(t, store, status.Revision, "granularity-finish-1")

	if status.CumulativeAttempts != 0 {
		t.Errorf("CumulativeAttempts = %d after an interrupted call that delivered increment, want 0", status.CumulativeAttempts)
	}
	if status.CumulativeChangedLines == 0 {
		t.Error("CumulativeChangedLines = 0; the delivered increment was not charged")
	}
	if status.DecisionRequired {
		t.Error("DecisionRequired after one delivering call")
	}

	status = beginRuntimeAttempt(t, store, status.Revision, "granularity-begin-2")
	appendRuntimeLedgerFile(t, repo, "second slice of the cutover\n")
	status = interruptRuntimeAttempt(t, store, status.Revision, "granularity-finish-2")

	if status.DecisionRequired {
		t.Errorf("DecisionRequired after two delivering calls on a %d-attempt objective; the unit exhausted by accounting", 2)
	}
	if status.NextAction != RuntimeActionBegin {
		t.Errorf("NextAction = %q after a delivering call, want %q", status.NextAction, RuntimeActionBegin)
	}
}

// TestRecoverableSetupFailuresDoNotConsumeAcceptanceAllowance proves that
// recoverable setup failures invalidated before acceptance preserve the
// objective allowance while retaining their global attempt history.
func TestRecoverableSetupFailuresDoNotConsumeAcceptanceAllowance(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	store, err := OpenRuntimeStore(context.Background(), repo, "acceptance-setup-budget")
	if err != nil {
		t.Fatal(err)
	}

	const (
		setupWorkUnit = "environment-cwd-and-harness-setup"
		evidenceGoal  = "open acceptance after recoverable setup failures"
		maxAttempts   = 2
	)
	setupFailures := []struct {
		beginID   string
		finishID  string
		diagnosis string
	}{
		{
			beginID: "environment-cwd-setup-begin", finishID: "environment-cwd-setup-finish",
			diagnosis: "environment and cwd setup failed before acceptance",
		},
		{
			beginID: "harness-setup-begin", finishID: "harness-setup-finish",
			diagnosis: "harness setup failed before acceptance",
		},
	}

	expected := ""
	for index, setup := range setupFailures {
		started, err := store.Begin(context.Background(), BeginAttemptRequest{
			ExpectedRevision: expected, RequestID: setup.beginID, WorkUnit: setupWorkUnit,
			EvidenceGoal: evidenceGoal, MaxAttempts: maxAttempts, MaxChangedLines: 20,
		})
		if err != nil {
			t.Fatalf("record setup failure %d: %v", index+1, err)
		}
		finished, err := store.Finish(context.Background(), FinishAttemptRequest{
			ExpectedRevision: started.Revision, RequestID: setup.finishID, Outcome: AttemptFailed,
			EvidenceRevision: runtimeTestHash(byte('a' + index)), Diagnosis: setup.diagnosis,
			HarnessDisposition: HarnessInvalidated, CleanupEvidence: "recoverable setup cleanup completed",
			ProcessEvidence: "setup process scan found no surviving descendants",
		})
		if err != nil {
			t.Fatalf("finish setup failure %d: %v", index+1, err)
		}
		expected = finished.Revision
	}

	acceptance, err := store.Begin(context.Background(), BeginAttemptRequest{
		ExpectedRevision: expected, RequestID: "acceptance-begin", WorkUnit: setupWorkUnit,
		EvidenceGoal: evidenceGoal, MaxAttempts: maxAttempts, MaxChangedLines: 20,
	})
	if err != nil {
		t.Fatalf("first acceptance attempt after recoverable setup failures = %v, want acceptance allowance ordinal 1/%d", err, maxAttempts)
	}
	if acceptance.ActiveAttempt == nil {
		t.Fatal("first acceptance allowance has no active attempt")
	}
	if acceptance.CumulativeAttempts != 1 || acceptance.Objective == nil || acceptance.Objective.MaxAttempts != maxAttempts {
		t.Fatalf("first acceptance allowance = %#v, want cumulative allowance 1/%d", acceptance, maxAttempts)
	}
	if acceptance.ActiveAttempt.Ordinal != 3 {
		t.Errorf("ActiveAttempt.Ordinal = %d, want global ordinal 3", acceptance.ActiveAttempt.Ordinal)
	}
	if acceptance.LifetimeAttempts != 3 {
		t.Errorf("LifetimeAttempts = %d, want 3; setup failures remain in global history", acceptance.LifetimeAttempts)
	}
}

func TestRuntimeAttemptRefundEligibility(t *testing.T) {
	tests := []struct {
		name               string
		outcome            AttemptOutcome
		changedLines       int
		harnessDisposition HarnessDisposition
		want               bool
	}{
		{name: "interrupted increment with reused harness", outcome: AttemptInterrupted, changedLines: 1, harnessDisposition: HarnessReused, want: true},
		{name: "interrupted without increment", outcome: AttemptInterrupted, harnessDisposition: HarnessReused, want: false},
		{name: "failed invalidated setup without increment", outcome: AttemptFailed, harnessDisposition: HarnessInvalidated, want: true},
		{name: "failed reused harness without increment", outcome: AttemptFailed, harnessDisposition: HarnessReused, want: false},
		{name: "failed invalidated setup with increment", outcome: AttemptFailed, changedLines: 1, harnessDisposition: HarnessInvalidated, want: false},
		{name: "passed", outcome: AttemptPassed, harnessDisposition: HarnessInvalidated, want: false},
		{name: "unknown", outcome: AttemptOutcome("unknown"), harnessDisposition: HarnessInvalidated, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := runtimeAttemptRefundEligible(tt.outcome, tt.changedLines, tt.harnessDisposition); got != tt.want {
				t.Errorf("runtimeAttemptRefundEligible(%q, %d, %q) = %t, want %t", tt.outcome, tt.changedLines, tt.harnessDisposition, got, tt.want)
			}
		})
	}
}

// TestInterruptedCallThatDeliveredNothingStillSpendsTheBudget pins the other
// half: a refund is earned by delivering, never granted for free.
func TestInterruptedCallThatDeliveredNothingStillSpendsTheBudget(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	store, err := OpenRuntimeStore(context.Background(), repo, "budget-granularity-empty")
	if err != nil {
		t.Fatal(err)
	}

	status := beginRuntimeAttempt(t, store, "", "empty-begin-1")
	status = interruptRuntimeAttempt(t, store, status.Revision, "empty-finish-1")

	if status.CumulativeAttempts != 1 {
		t.Errorf("CumulativeAttempts = %d after an interrupted call that delivered nothing, want 1", status.CumulativeAttempts)
	}
	if status.LifetimeAttempts != 1 {
		t.Errorf("LifetimeAttempts = %d, want 1; the lifetime counter is never refunded", status.LifetimeAttempts)
	}
}

// TestRefundsAreCappedAtTheConfiguredAttemptCeiling pins the bound: an
// objective earns back at most MaxAttempts calls, so it spends at most twice
// what the operator configured and max_attempts still escalates.
func TestRefundsAreCappedAtTheConfiguredAttemptCeiling(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	store, err := OpenRuntimeStore(context.Background(), repo, "budget-refund-cap")
	if err != nil {
		t.Fatal(err)
	}

	status := RuntimeStatus{}
	expected := ""
	for call := 1; call <= 4; call++ {
		status = beginRuntimeAttempt(t, store, expected, fmt.Sprintf("cap-begin-%d", call))
		appendRuntimeLedgerFile(t, repo, fmt.Sprintf("slice %d\n", call))
		status = interruptRuntimeAttempt(t, store, status.Revision, fmt.Sprintf("cap-finish-%d", call))
		expected = status.Revision
		if call < 4 && status.DecisionRequired {
			t.Fatalf("call %d reached decision-required before the 2x ceiling", call)
		}
	}

	if !status.DecisionRequired {
		t.Errorf("four delivering calls on a 2-attempt objective did not reach decision-required; max_attempts no longer escalates")
	}
	if status.LifetimeAttempts != 4 {
		t.Errorf("LifetimeAttempts = %d, want 4; every call that ran must stay recorded", status.LifetimeAttempts)
	}
}
