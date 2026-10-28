package container

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/bezbayana777/trip_go/internal/infra/postgres"
	repoTrip "github.com/bezbayana777/trip_go/internal/repository/trip"
	"github.com/bezbayana777/trip_go/internal/usecase/trip/create"
	"github.com/bezbayana777/trip_go/internal/usecase/trip/finish"
	"github.com/bezbayana777/trip_go/internal/usecase/trip/get"
)

type Container struct {
	DBPool         *pgxpool.Pool
	CreateUsecase  *create.Usecase
	GetUsecase     *get.Usecase
	FinishUsecase  *finish.Usecase
}

func New(ctx context.Context, databaseURL string) (*Container, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to create db pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("failed to ping db: %w", err)
	}

	tripRepo := repoTrip.New(pool)
	txManager := postgres.NewTxManager(pool)

	return &Container{
		DBPool:         pool,
		CreateUsecase:  create.New(tripRepo, txManager),
		GetUsecase:     get.New(tripRepo),
		FinishUsecase:  finish.New(tripRepo, txManager),
	}, nil
}