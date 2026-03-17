package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"gitlab.com/tiny-services/multiverse-bot/internal/domain"
)

type Store struct {
	db *sql.DB
}

func New(path string) (*Store, error) {
	if path != ":memory:" {
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return nil, fmt.Errorf("creating db directory: %w", err)
		}
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("opening sqlite: %w", err)
	}

	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		return nil, fmt.Errorf("enabling WAL mode: %w", err)
	}

	if err := migrate(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrating: %w", err)
	}

	return &Store{db: db}, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func migrate(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS subscriptions (
			id           INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id      INTEGER NOT NULL,
			channel_id   TEXT NOT NULL,
			channel_name TEXT NOT NULL DEFAULT '',
			created_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(user_id, channel_id)
		);
		CREATE INDEX IF NOT EXISTS idx_sub_user ON subscriptions(user_id);
		CREATE INDEX IF NOT EXISTS idx_sub_channel ON subscriptions(channel_id);

		CREATE TABLE IF NOT EXISTS seen_videos (
			user_id    INTEGER NOT NULL,
			channel_id TEXT NOT NULL,
			video_id   TEXT NOT NULL,
			seen_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (user_id, channel_id, video_id)
		);
	`)
	return err
}

func (s *Store) AddSubscription(ctx context.Context, userID int64, channelID, channelName string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO subscriptions (user_id, channel_id, channel_name) VALUES (?, ?, ?)`,
		userID, channelID, channelName,
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return domain.ErrAlreadySubscribed
		}
		return err
	}
	return nil
}

func (s *Store) RemoveSubscription(ctx context.Context, userID int64, channelID string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	result, err := tx.ExecContext(ctx,
		`DELETE FROM subscriptions WHERE user_id = ? AND channel_id = ?`,
		userID, channelID,
	)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return domain.ErrNotSubscribed
	}

	var count int
	if err := tx.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM subscriptions WHERE channel_id = ?`,
		channelID,
	).Scan(&count); err != nil {
		return err
	}

	if count == 0 {
		if _, err := tx.ExecContext(ctx,
			`DELETE FROM seen_videos WHERE channel_id = ?`,
			channelID,
		); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (s *Store) GetSubscriptions(ctx context.Context, userID int64) ([]domain.Subscription, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, user_id, channel_id, channel_name, created_at FROM subscriptions WHERE user_id = ? ORDER BY created_at`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var subs []domain.Subscription
	for rows.Next() {
		var sub domain.Subscription
		var createdAt string
		if err := rows.Scan(&sub.ID, &sub.UserID, &sub.ChannelID, &sub.ChannelName, &createdAt); err != nil {
			return nil, err
		}
		sub.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
		subs = append(subs, sub)
	}
	return subs, rows.Err()
}

func (s *Store) CountSubscriptions(ctx context.Context, userID int64) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM subscriptions WHERE user_id = ?`,
		userID,
	).Scan(&count)
	return count, err
}

func (s *Store) GetAllUniqueChannels(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT DISTINCT channel_id FROM subscriptions`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var channels []string
	for rows.Next() {
		var ch string
		if err := rows.Scan(&ch); err != nil {
			return nil, err
		}
		channels = append(channels, ch)
	}
	return channels, rows.Err()
}

func (s *Store) GetSubscribers(ctx context.Context, channelID string) ([]int64, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT user_id FROM subscriptions WHERE channel_id = ?`,
		channelID,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var userIDs []int64
	for rows.Next() {
		var uid int64
		if err := rows.Scan(&uid); err != nil {
			return nil, err
		}
		userIDs = append(userIDs, uid)
	}
	return userIDs, rows.Err()
}

func (s *Store) HasSeenVideo(ctx context.Context, userID int64, channelID, videoID string) (bool, error) {
	var exists bool
	err := s.db.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM seen_videos WHERE user_id = ? AND channel_id = ? AND video_id = ?)`,
		userID, channelID, videoID,
	).Scan(&exists)
	return exists, err
}

func (s *Store) MarkVideoSeen(ctx context.Context, userID int64, channelID, videoID string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO seen_videos (user_id, channel_id, video_id) VALUES (?, ?, ?)`,
		userID, channelID, videoID,
	)
	return err
}

func (s *Store) CleanupExpiredSeen(ctx context.Context) (int64, error) {
	result, err := s.db.ExecContext(ctx,
		`DELETE FROM seen_videos WHERE seen_at < datetime('now', '-30 days')`,
	)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}
