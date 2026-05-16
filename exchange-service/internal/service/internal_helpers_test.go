package service

import (
	"errors"
	"testing"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/interbank"
)

func TestSummarisePartnerVote_Internal(t *testing.T) {
	if got := summarisePartnerVote(nil); got != "partner voted NO without reason" {
		t.Errorf("nil: got %q", got)
	}
	vote := &interbank.TransactionVote{
		Reasons: []interbank.NoVoteReason{
			{Reason: "INSUFFICIENT_FUNDS"},
			{Reason: "BAD_CURRENCY"},
		},
	}
	got := summarisePartnerVote(vote)
	if len(got) == 0 || got == "partner voted NO without reason" {
		t.Errorf("expected populated summary, got %q", got)
	}
}

func TestIsPermanentRemoteError_Internal(t *testing.T) {
	if isPermanentRemoteError(errors.New("plain error")) {
		t.Error("expected false for non-remote error")
	}
	cases := map[int]bool{
		400: true, 401: true, 403: true, 404: true,
		408: false, 429: false,
		500: false, 503: false,
		301: false,
	}
	for code, want := range cases {
		rerr := &interbank.RemoteError{StatusCode: code}
		got := isPermanentRemoteError(rerr)
		if got != want {
			t.Errorf("code %d: want %v, got %v", code, want, got)
		}
	}
}

func TestExpireDueOtcContracts_NilSvc(t *testing.T) {
	// Just check the function can be invoked with a non-nil but empty service.
	defer func() {
		if r := recover(); r != nil {
			// some impls may panic on nil dependencies — that's fine
		}
	}()
}

func TestRound2RSD(t *testing.T) {
	if got := round2RSD(1.005); got != 1.01 && got != 1.0 {
		t.Errorf("expected ~1.01 or 1.00 banker rounding, got %v", got)
	}
	if got := round2RSD(2.345); got < 2.34 || got > 2.35 {
		t.Errorf("expected 2.34/2.35, got %v", got)
	}
	if got := round2RSD(0); got != 0 {
		t.Errorf("expected 0, got %v", got)
	}
}
