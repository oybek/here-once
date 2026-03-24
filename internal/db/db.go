package db

import (
	"context"

	"github.com/jackc/pgx/v5"
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
		`INSERT INTO here_onces (user_id, lat, lon, note, photo_ids, created)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING id`,
		h.UserID, h.Lat, h.Lon, h.Note, h.PhotoIDs, h.Created,
	).Scan(&id)
	if err != nil {
		return 0, err
	}
	return id, nil
}

func (s *Store) RandomHereOnceByUser(ctx context.Context, userID int64) (*model.HereOnce, error) {
	var h model.HereOnce
	err := s.pool.QueryRow(
		ctx,
		`SELECT id, user_id, lat, lon, note, photo_ids, created
		 FROM here_onces
		 WHERE user_id = $1
		 ORDER BY random()
		 LIMIT 1`,
		userID,
	).Scan(&h.ID, &h.UserID, &h.Lat, &h.Lon, &h.Note, &h.PhotoIDs, &h.Created)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &h, nil
}

func (s *Store) LatestHereOnceByUser(ctx context.Context, userID int64) (*model.HereOnce, error) {
	var h model.HereOnce
	err := s.pool.QueryRow(
		ctx,
		`SELECT id, user_id, lat, lon, note, photo_ids, created
		 FROM here_onces
		 WHERE user_id = $1
		 ORDER BY created DESC
		 LIMIT 1`,
		userID,
	).Scan(&h.ID, &h.UserID, &h.Lat, &h.Lon, &h.Note, &h.PhotoIDs, &h.Created)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &h, nil
}
