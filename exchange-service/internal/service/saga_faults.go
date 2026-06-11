package service

import "time"

// SagaFaultConfig drives deterministic fault injection for the SAGA test suite
// (files/SAGA.md). It is ONLY constructed when the SAGA_FAULT_HOOKS env flag is
// set; cmd/server refuses to boot with the flag enabled in release mode. Step
// numbers are 1-based forward steps (F1..F5); compensators share the number
// (C1..C5).
//
// All methods are nil-safe: a nil *SagaFaultConfig injects nothing, so the
// production path (Run -> RunWithFaults(nil)) is byte-for-byte unaffected.
type SagaFaultConfig struct {
	// ForceFailStep is the 1-based forward step to fail (0 = none).
	ForceFailStep int
	// ForceFailAfter fails the step AFTER its side effects commit (default is
	// before, i.e. the step is a no-op that errors).
	ForceFailAfter bool
	// CompensateFailStep is the 1-based compensator to fail (0 = none).
	CompensateFailStep int
	// CompensateFailTimes is how many consecutive compensator attempts fail
	// before one is allowed to succeed.
	CompensateFailTimes int
	// InjectDelays maps a forward step number to a sleep applied before it runs.
	InjectDelays map[int]time.Duration

	// compensateAttempts counts forced compensator failures consumed so far.
	compensateAttempts map[int]int
}

func (f *SagaFaultConfig) delayFor(step int) time.Duration {
	if f == nil {
		return 0
	}
	return f.InjectDelays[step]
}

func (f *SagaFaultConfig) shouldFailForward(step int) bool {
	return f != nil && f.ForceFailStep == step
}

func (f *SagaFaultConfig) forceFailAfter() bool {
	return f != nil && f.ForceFailAfter
}

// shouldFailCompensation reports whether the compensator for step must be forced
// to fail on this attempt, consuming one configured failure. After
// CompensateFailTimes calls it returns false so a later retry can succeed.
func (f *SagaFaultConfig) shouldFailCompensation(step int) bool {
	if f == nil || f.CompensateFailStep != step || f.CompensateFailTimes <= 0 {
		return false
	}
	if f.compensateAttempts == nil {
		f.compensateAttempts = make(map[int]int)
	}
	if f.compensateAttempts[step] >= f.CompensateFailTimes {
		return false
	}
	f.compensateAttempts[step]++
	return true
}
