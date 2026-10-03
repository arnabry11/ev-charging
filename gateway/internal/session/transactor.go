package session

import (
	"context"

	"github.com/arnabry11/ev-charging/gateway/internal/store"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type unitOfWork interface {
	Within(context.Context, func(repository) error) error
}

type Transactor struct {
	db interface {
		Begin(context.Context) (pgx.Tx, error)
	}
}

func NewTransactor(pool *pgxpool.Pool) Transactor {
	return Transactor{db: pool}
}

func (t Transactor) Within(ctx context.Context, fn func(repository) error) error {
	tx, err := t.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := fn(store.New(tx)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
