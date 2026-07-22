package domain

import "testing"

func TestStateMachineRejectsInvalidTransitions(t *testing.T) {
	tests := []struct {
		from State
		to   State
		want bool
	}{
		{StatePending, StatePaymentPending, true},
		{StatePaymentPending, StatePaid, true},
		{StatePaid, StateBookingConfirmed, true},
		{StateBookingConfirmed, StateCompleted, false},
		{StateCompleted, StateRefunded, false},
		{StatePaid, StateCancelled, false},
	}
	for _, test := range tests {
		t.Run(string(test.from)+"_to_"+string(test.to), func(t *testing.T) {
			if got := (Order{State: test.from}).CanTransition(test.to); got != test.want {
				t.Fatalf("CanTransition=%v, want %v", got, test.want)
			}
		})
	}
}
