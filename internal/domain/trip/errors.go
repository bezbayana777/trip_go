package trip

import "errors"

var (
	ErrTripNotFound    = errors.New("trip not found")
	ErrTripCompleted   = errors.New("trip already completed")
	ErrDriverBusy      = errors.New("driver already has an active trip")
	ErrInvalidPosition = errors.New("invalid position")
)
