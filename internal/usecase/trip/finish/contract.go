//go:generate mockgen -source ${GOFILE} -destination mocks_test.go -package ${GOPACKAGE}_test
package finish

import (
	"context"
	"github.com/google/uuid"
)

type (
	tripRepository interface {
		FinishTrip(ctx context.Context, id uuid.UUID) error
	}

	txManager interface {
		Do(ctx context.Context, fn func(ctx context.Context) error) error
	}
)