package finish

import (
	"context"

	"github.com/google/uuid"
)

//go:generate mockgen -source ${GOFILE} -destination mocks_test.go -package ${GOPACKAGE}_test
type Usecase interface {
	Execute(ctx context.Context, id uuid.UUID) error
}