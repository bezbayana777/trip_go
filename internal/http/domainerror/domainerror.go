package domainerror

import (
	"errors"
	"net/http"

	"github.com/bezbayana777/trip_go/internal/domain/trip"
	generated "github.com/bezbayana777/trip_go/internal/generated" 
)

func Problem(err error) (int, generated.Problem) {
	// 1. Ошибки водителя / конфликты (409)
	if errors.Is(err, trip.ErrDriverBusy) {
		return http.StatusConflict, generated.Problem{
			Type:   "https://tripgo.example/problems/driver-busy",
			Title:  "Driver busy",
			Status: http.StatusConflict,
			Code:   "driver_busy",
		}
	}

	if errors.Is(err, trip.ErrTripCompleted) {
		return http.StatusConflict, generated.Problem{
			Type:   "https://tripgo.example/problems/trip-completed",
			Title:  "Trip completed",
			Status: http.StatusConflict,
			Code:   "trip_completed",
		}
	}

	// 2. Ошибки «не найдено» (404)
	if errors.Is(err, trip.ErrTripNotFound) {
		return http.StatusNotFound, generated.Problem{
			Type:   "https://tripgo.example/problems/trip-not-found",
			Title:  "Trip not found",
			Status: http.StatusNotFound,
			Code:   "trip_not_found",
		}
	}

	// 3. Общая внутренняя ошибка по умолчанию (500)
	return http.StatusInternalServerError, generated.Problem{
		Type:   "https://tripgo.example/problems/internal-error",
		Title:  "Internal Server Error",
		Status: http.StatusInternalServerError,
		Code:   "internal_error",
	}
}