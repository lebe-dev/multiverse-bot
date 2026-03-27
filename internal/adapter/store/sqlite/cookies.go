package sqlite

import (
	"context"
	"database/sql"
	"errors"
)

func (s *Store) SaveCookies(ctx context.Context, platform string, data []byte) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO cookies (platform, data, updated_at) VALUES (?, ?, CURRENT_TIMESTAMP)
		 ON CONFLICT(platform) DO UPDATE SET data = excluded.data, updated_at = CURRENT_TIMESTAMP`,
		platform, data,
	)
	return err
}

func (s *Store) GetCookies(ctx context.Context, platform string) ([]byte, error) {
	var data []byte
	err := s.db.QueryRowContext(ctx,
		`SELECT data FROM cookies WHERE platform = ?`, platform,
	).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return data, err
}

func (s *Store) DeleteCookies(ctx context.Context, platform string) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM cookies WHERE platform = ?`, platform,
	)
	return err
}

func (s *Store) ListCookiePlatforms(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT platform FROM cookies ORDER BY platform`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var platforms []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		platforms = append(platforms, p)
	}
	return platforms, rows.Err()
}
