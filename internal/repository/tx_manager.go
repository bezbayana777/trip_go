

package repository

import (
	"context"
	"fmt"

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

	txOptions := pgx.TxOptions{
		IsoLevel: pgx.ReadCommitted,
	}

	tx, err := m.pool.BeginTx(ctx, txOptions)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	ctxWithTx := context.WithValue(ctx, txKey, tx)

	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback(ctx)
			panic(p) 
		}
	}()

	if err := fn(ctxWithTx); err != nil {
		if rollbackErr := tx.Rollback(ctx); rollbackErr != nil {
			return fmt.Errorf("tx execution error: %v, rollback error: %v", err, rollbackErr)
		}
		return err
	}

	if err := tx.Commit(ctx); err != nil {
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