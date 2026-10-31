package repository

import (
	"context"
	"fmt"

	"github.com/bezbayana777/trip_go/internal/db"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type contextKey string

const txKey contextKey = "tx_key"

type TxManager interface {
	Do(ctx context.Context, fn func(ctx context.Context) error) error
}

type pgTxManager struct {
	pool *pgxpool.Pool
}

func NewTxManager(pool *pgxpool.Pool) TxManager {
	return &pgTxManager{pool: pool}
}

func (m *pgTxManager) Do(ctx context.Context, fn func(ctx context.Context) error) error {
	if _, ok := ctx.Value(txKey).(pgx.Tx); ok {
		return fn(ctx)
	}

	beginCtx, cancelBegin := db.WithQueryTimeout(ctx)
	tx, err := m.pool.BeginTx(beginCtx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	cancelBegin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	rollback := func() error {
		rbCtx, cancel := db.WithQueryTimeout(context.WithoutCancel(ctx))
		defer cancel()
		return tx.Rollback(rbCtx)
	}

	ctxWithTx := context.WithValue(ctx, txKey, tx)

	defer func() {
		if p := recover(); p != nil {
			_ = rollback()
			panic(p)
		}
	}()

	if err := fn(ctxWithTx); err != nil {
		if rbErr := rollback(); rbErr != nil {
			return fmt.Errorf("tx execution error: %w, rollback error: %v", err, rbErr)
		}
		return err
	}

	commitCtx, cancelCommit := db.WithQueryTimeout(ctx)
	defer cancelCommit()
	if err := tx.Commit(commitCtx); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

func getDB(ctx context.Context, pool *pgxpool.Pool) DBops {
	if tx, ok := ctx.Value(txKey).(pgx.Tx); ok {
		return tx
	}
	return pool
}