package trip_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/bezbayana777/trip_go/internal/domain/trip"
)

func TestTrip_Finish(t *testing.T) {
	t.Run("success finishing active trip", func(t *testing.T) {
		tr := &trip.Trip{
			Status: trip.StatusActive,
		}
		now := time.Now()
		
		err := tr.Finish(now)

		assert.NoError(t, err)
		assert.Equal(t, trip.StatusCompleted, tr.Status)
		assert.Equal(t, &now, tr.FinishedAt)
	})

	t.Run("error when trip already completed", func(t *testing.T) {
		tr := &trip.Trip{
			Status: trip.StatusCompleted,
		}
		
		err := tr.Finish(time.Now())

		assert.ErrorIs(t, err, trip.ErrTripCompleted)
	})
}