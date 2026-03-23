package db

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/oybek/ho/internal/model"
)

type Store struct {
	pool *pgxpool.Pool
}

func New(ctx context.Context, connString string) (*Store, error) {
	pool, err := pgxpool.New(ctx, connString)
	if err != nil {
		return nil, err
	}
	return &Store{pool: pool}, nil
}

func (s *Store) Close() {
	if s.pool != nil {
		s.pool.Close()
	}
}

func (s *Store) InsertHereOnce(ctx context.Context, h *model.HereOnce) (int64, error) {
	var id int64
	err := s.pool.QueryRow(
		ctx,
		`INSERT INTO here_onces (lat, lon, note, photo_ids, created)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id`,
		h.Lat, h.Lon, h.Note, h.PhotoIDs, h.Created,
	).Scan(&id)
	if err != nil {
		return 0, err
	}
	return id, nil
}
