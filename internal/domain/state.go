package domain

import "errors"

var ErrInvalidTransition = errors.New("invalid task state transition")

func CanTransition(from, to Status) bool {
	switch from {
	case StatusQueued:
		return to == StatusRunning || to == StatusCancelled || to == StatusError
	case StatusRunning:
		return to == StatusSucceeded || to == StatusFailed || to == StatusTimeout || to == StatusCancelled || to == StatusError || to == StatusQueued
	default:
		return false
	}
}
