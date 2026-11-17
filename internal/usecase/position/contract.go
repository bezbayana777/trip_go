package position

import (
	"context"
	"time"

	"github.com/google/uuid"

	domain "github.com/bezbayana777/trip_go/internal/domain/position"
)

type Repository interface {
	Save(context.Context, uuid.UUID, domain.Position) (domain.Position, error)
	List(context.Context, uuid.UUID) ([]domain.Position, error)
}
type TxManager interface {
	Do(context.Context, func(context.Context) error) error
}
type (
	Queue interface{ Cancel(uuid.UUID) }
	Clock interface{ Now() time.Time }
)
