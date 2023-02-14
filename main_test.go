package main

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

var ErrEventNotFound = fmt.Errorf("event not found")
var ErrNoTransitionForEvent = fmt.Errorf("no transition for event")

type Repository interface {
}

type State string

const (
	StatePending             State = "pending"
	StateAuthorized          State = "authorized"
	StatePartiallyAuthorized State = "partially_authorized"
	StateCaptured            State = "captured"
	StateVoided              State = "voided"
)

type Event struct {
	Transitions []Transition
}

type Transition struct {
	From State
	To   State
	// Guard is a function that returns true if the transition is allowed
	Guard func(args ...any) bool

	// On is a function that is called when the transition is triggered
	// if the function returns an error, the transition is not executed
	On func(args ...any) error

	// After is a function that is called after the transition
	After func(args ...any) error
}

type StateMachine struct {
	events       map[string]Event
	currentState State
}

type Options struct {
	// or initial state
	CurrentState State
}

func NewStateMachine(opts Options) *StateMachine {
	return &StateMachine{
		events:       make(map[string]Event),
		currentState: opts.CurrentState,
	}
}

func (sm *StateMachine) SetEvents(events map[string]Event) {
	sm.events = events
}

// Fire triggers the event and changes the state of the subject by
// executing the transition. It executes only the first transition.
func (sm *StateMachine) Fire(name string, args ...any) error {
	event, ok := sm.events[name]
	if !ok {
		return ErrEventNotFound
	}

	// iterate over transitions
	// check if current state is in the list of from states
	// if yes, then change the state to the to state
	// if no, then return error

	// we have to load current state of the subject from the database
	// and lock the row (SELECT ... FOR UPDATE)

	// we have to check if the transition is allowed
	// if not, then return error and rollback the transaction

	// we have to call the On function
	// if it returns an error, then return error and rollback the transaction
	// otherwise, we update the subject state and commit the transaction

	// we have to call the After function

	for _, transition := range event.Transitions {
		if sm.currentState == transition.From {
			if transition.Guard != nil && !transition.Guard(args...) {
				continue
			}

			currentState := sm.currentState

			sm.currentState = transition.To

			if transition.On != nil {
				err := transition.On(args...)
				if err != nil {
					sm.currentState = currentState
					return fmt.Errorf("error during transition from %s to %s: %w", currentState, transition.To, err)
				}
			}

			if transition.After != nil {
				err := transition.After(args...)
				if err != nil {
					return fmt.Errorf("error calling after function: %w", err)
				}
			}

			return nil
		}
	}

	return fmt.Errorf("event %s: %w", name, ErrNoTransitionForEvent)
}

func (sm *StateMachine) State() State {
	return sm.currentState
}

type Transfer struct {
	ID               string
	AuthorizedAmount int
	CapturedAmount   int
	VoidedAmount     int
	Status           State
}

// Update updates the transfer in the database using transactional
// operations
func (t *Transfer) Update() error {
	return nil
}

func TestFSM(t *testing.T) {
	xfr := Transfer{
		ID: "xfr",
	}

	sm := NewStateMachine(Options{
		CurrentState: StatePending,
	})

	sm.SetEvents(map[string]Event{
		"authorize": Event{
			Transitions: []Transition{
				Transition{
					From: StatePending,
					To:   StateAuthorized,
					On: func(args ...any) error {
						if len(args) == 0 {
							return nil
						}
						amount := args[0].(int)

						xfr.AuthorizedAmount = amount

						return nil
					},
					After: func(...any) error {
						// let's say we produce event here
						fmt.Println("produce authorize event")
						return nil
					},
				},
			},
		},
		"capture": Event{
			Transitions: []Transition{
				Transition{
					From: StateAuthorized,
					To:   StateCaptured,
				},
			},
		},
		"void": Event{
			Transitions: []Transition{
				Transition{
					From: StateAuthorized,
					To:   StatePartiallyAuthorized,
					Guard: func(args ...any) bool {
						if len(args) == 0 {
							return false
						}
						amount := args[0].(int)

						if amount < xfr.AuthorizedAmount {
							return true
						}

						return false
					},
					On: func(args ...any) error {
						if len(args) == 0 {
							return nil
						}
						amount := args[0].(int)

						xfr.VoidedAmount += amount
						xfr.AuthorizedAmount -= amount

						return nil
					},
				},
				Transition{
					From: StateAuthorized,
					To:   StateVoided,
					Guard: func(args ...any) bool {
						var amount int
						if len(args) != 0 {
							amount = args[0].(int)
						} else {
							amount = xfr.AuthorizedAmount
						}

						if amount == xfr.AuthorizedAmount {
							return true
						}

						return false
					},
					On: func(args ...any) error {
						var amount int
						if len(args) != 0 {
							amount = args[0].(int)
						} else {
							amount = xfr.AuthorizedAmount
						}

						xfr.VoidedAmount += amount
						xfr.AuthorizedAmount -= amount

						return nil
					},
				},
			},
		},
	})

	err := sm.Fire("authorize", 100)
	require.NoError(t, err)

	fmt.Printf("%+v\n", xfr)

	require.Equal(t, StateAuthorized, sm.State())

	err = sm.Fire("void", 50)
	require.NoError(t, err)

	fmt.Printf("%+v\n", xfr)

	require.Equal(t, StatePartiallyAuthorized, sm.State())

	err = sm.Fire("void", 150)
	require.NoError(t, err)
}
