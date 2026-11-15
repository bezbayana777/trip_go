package domainerror

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/bezbayana777/trip_go/internal/domain/trip"
	generated "github.com/bezbayana777/trip_go/internal/generated"
)

type ValidationError struct{ Field string }

func (e *ValidationError) Error() string { return fmt.Sprintf("invalid field: %s", e.Field) }

func Problem(err error) (int, generated.Problem) {
	var validation *ValidationError
	if errors.As(err, &validation) {
		return http.StatusBadRequest, generated.Problem{Type: "https://tripgo.example/problems/invalid-request", Title: "Invalid request", Status: http.StatusBadRequest, Code: "invalid_request", Detail: stringPointer("invalid field: " + validation.Field)}
	}
	// 1. Ошибки водителя / конфликты (409)
	if errors.Is(err, trip.ErrDriverBusy) {
		return http.StatusConflict, generated.Problem{
			Type:   "https://tripgo.example/problems/driver-busy",
			Title:  "Driver busy",
			Status: http.StatusConflict,
			Code:   "driver_busy",
			Detail: stringPointer("driver already has an active trip"),
		}
	}

	if errors.Is(err, trip.ErrTripCompleted) {
		return http.StatusConflict, generated.Problem{
			Type:   "https://tripgo.example/problems/trip-completed",
			Title:  "Trip completed",
			Status: http.StatusConflict,
			Code:   "trip_completed",
			Detail: stringPointer("trip has already been completed"),
		}
	}

	// 2. Ошибки «не найдено» (404)
	if errors.Is(err, trip.ErrTripNotFound) {
		return http.StatusNotFound, generated.Problem{
			Type:   "https://tripgo.example/problems/trip-not-found",
			Title:  "Trip not found",
			Status: http.StatusNotFound,
			Code:   "trip_not_found",
			Detail: stringPointer("trip does not exist"),
		}
	}

	// 3. Общая внутренняя ошибка по умолчанию (500)
	return http.StatusInternalServerError, generated.Problem{
		Type:   "https://tripgo.example/problems/internal-error",
		Title:  "Internal Server Error",
		Status: http.StatusInternalServerError,
		Code:   "internal_error",
		Detail: stringPointer("an unexpected error occurred"),
	}
}

func stringPointer(s string) *string { return &s }

func ProblemForInternal() generated.Problem {
	_, problem := Problem(errors.New("internal"))
	return problem
}
