// Package sqlstore implements model.Store backed by a SQLite database.
package sqlstore

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"time"

	"github.com/samuel/demy/internal/model"
	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// SQLiteStore is a model.Store backed by SQLite.
type SQLiteStore struct {
	db *sql.DB
}

// Open opens (or creates) a SQLite database at path, applies any pending
// migrations, and returns a ready-to-use SQLiteStore.
func Open(ctx context.Context, path string) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("sqlstore: open db: %w", err)
	}

	if _, err = db.ExecContext(ctx, `PRAGMA journal_mode=WAL; PRAGMA foreign_keys=ON;`); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("sqlstore: set pragmas: %w", err)
	}

	s := &SQLiteStore{db: db}
	if err = s.migrate(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

// Close closes the underlying database connection.
func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

// migrate creates the migrations tracking table and applies any SQL files
// under migrations/ that have not already been applied.
func (s *SQLiteStore) migrate(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			name TEXT PRIMARY KEY,
			applied_at DATETIME NOT NULL
		)`)
	if err != nil {
		return fmt.Errorf("sqlstore: create migrations table: %w", err)
	}

	entries, err := fs.ReadDir(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("sqlstore: read migrations dir: %w", err)
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		var count int
		if err = s.db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM schema_migrations WHERE name = ?`, name,
		).Scan(&count); err != nil {
			return fmt.Errorf("sqlstore: check migration %s: %w", name, err)
		}
		if count > 0 {
			continue
		}

		data, err := migrationsFS.ReadFile("migrations/" + name)
		if err != nil {
			return fmt.Errorf("sqlstore: read migration %s: %w", name, err)
		}

		if _, err = s.db.ExecContext(ctx, string(data)); err != nil {
			return fmt.Errorf("sqlstore: apply migration %s: %w", name, err)
		}

		if _, err = s.db.ExecContext(ctx,
			`INSERT INTO schema_migrations (name, applied_at) VALUES (?, ?)`,
			name, time.Now().UTC().Format(time.RFC3339Nano),
		); err != nil {
			return fmt.Errorf("sqlstore: record migration %s: %w", name, err)
		}
	}
	return nil
}

// ─── helpers ────────────────────────────────────────────────────────────────

func formatTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}

func parseTime(s string) (time.Time, error) {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		// fall back to plain RFC3339
		t, err = time.Parse(time.RFC3339, s)
	}
	return t, err
}

func scanTime(s string) (time.Time, error) {
	return parseTime(s)
}

func nullTimeStr(t time.Time) sql.NullString {
	if t.IsZero() {
		return sql.NullString{}
	}
	return sql.NullString{String: formatTime(t), Valid: true}
}

func nullStrTime(ns sql.NullString) time.Time {
	if !ns.Valid {
		return time.Time{}
	}
	t, _ := parseTime(ns.String)
	return t
}

func marshalTransports(ts []string) string {
	if ts == nil {
		return "[]"
	}
	b, _ := json.Marshal(ts)
	return string(b)
}

func unmarshalTransports(s string) []string {
	var ts []string
	_ = json.Unmarshal([]byte(s), &ts)
	return ts
}

// ─── Users ──────────────────────────────────────────────────────────────────

func (s *SQLiteStore) CreateUser(ctx context.Context, u *model.User) error {
	var isAdmin int
	if u.IsAdmin {
		isAdmin = 1
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO users (id, display_name, email, is_admin, created_at)
		 VALUES (?, ?, ?, ?, ?)`,
		u.ID, u.DisplayName, u.Email, isAdmin, formatTime(u.CreatedAt),
	)
	if err != nil {
		return fmt.Errorf("sqlstore: CreateUser: %w", err)
	}
	return nil
}

func scanUser(row interface{ Scan(...any) error }) (*model.User, error) {
	var (
		u         model.User
		isAdmin   int
		createdAt string
	)
	if err := row.Scan(&u.ID, &u.DisplayName, &u.Email, &isAdmin, &createdAt); err != nil {
		return nil, err
	}
	u.IsAdmin = isAdmin != 0
	t, err := scanTime(createdAt)
	if err != nil {
		return nil, fmt.Errorf("sqlstore: parse user created_at: %w", err)
	}
	u.CreatedAt = t
	return &u, nil
}

func (s *SQLiteStore) GetUserByID(ctx context.Context, id string) (*model.User, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, display_name, email, is_admin, created_at FROM users WHERE id = ?`, id)
	u, err := scanUser(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, model.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("sqlstore: GetUserByID: %w", err)
	}
	return u, nil
}

func (s *SQLiteStore) GetUserByEmail(ctx context.Context, email string) (*model.User, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, display_name, email, is_admin, created_at FROM users WHERE email = ?`, email)
	u, err := scanUser(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, model.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("sqlstore: GetUserByEmail: %w", err)
	}
	return u, nil
}

func (s *SQLiteStore) UpdateUser(ctx context.Context, u *model.User) error {
	var isAdmin int
	if u.IsAdmin {
		isAdmin = 1
	}
	_, err := s.db.ExecContext(ctx,
		`UPDATE users SET display_name = ?, email = ?, is_admin = ? WHERE id = ?`,
		u.DisplayName, u.Email, isAdmin, u.ID,
	)
	if err != nil {
		return fmt.Errorf("sqlstore: UpdateUser: %w", err)
	}
	return nil
}

func (s *SQLiteStore) CountUsers(ctx context.Context) (int, error) {
	var n int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&n); err != nil {
		return 0, fmt.Errorf("sqlstore: CountUsers: %w", err)
	}
	return n, nil
}

func (s *SQLiteStore) ListUsers(ctx context.Context) ([]*model.User, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, display_name, email, is_admin, created_at FROM users ORDER BY created_at`)
	if err != nil {
		return nil, fmt.Errorf("sqlstore: ListUsers: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var users []*model.User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, fmt.Errorf("sqlstore: ListUsers scan: %w", err)
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

// ─── Passkeys ───────────────────────────────────────────────────────────────

func (s *SQLiteStore) CreatePasskey(ctx context.Context, pk *model.PasskeyCredential) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO passkey_credentials
		 (id, user_id, credential_id, public_key, aaguid, sign_count, transports, name, created_at, last_used_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		pk.ID, pk.UserID, pk.CredentialID, pk.PublicKey, pk.AAGUID,
		pk.SignCount, marshalTransports(pk.Transports), pk.Name,
		formatTime(pk.CreatedAt), formatTime(pk.LastUsedAt),
	)
	if err != nil {
		return fmt.Errorf("sqlstore: CreatePasskey: %w", err)
	}
	return nil
}

func scanPasskey(row interface{ Scan(...any) error }) (*model.PasskeyCredential, error) {
	var (
		pk         model.PasskeyCredential
		transports string
		createdAt  string
		lastUsedAt string
	)
	if err := row.Scan(
		&pk.ID, &pk.UserID, &pk.CredentialID, &pk.PublicKey, &pk.AAGUID,
		&pk.SignCount, &transports, &pk.Name, &createdAt, &lastUsedAt,
	); err != nil {
		return nil, err
	}
	pk.Transports = unmarshalTransports(transports)
	t, err := scanTime(createdAt)
	if err != nil {
		return nil, fmt.Errorf("sqlstore: parse passkey created_at: %w", err)
	}
	pk.CreatedAt = t
	t, err = scanTime(lastUsedAt)
	if err != nil {
		return nil, fmt.Errorf("sqlstore: parse passkey last_used_at: %w", err)
	}
	pk.LastUsedAt = t
	return &pk, nil
}

func (s *SQLiteStore) GetPasskeysByUserID(ctx context.Context, userID string) ([]*model.PasskeyCredential, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, user_id, credential_id, public_key, aaguid, sign_count, transports, name, created_at, last_used_at
		 FROM passkey_credentials WHERE user_id = ? ORDER BY created_at`, userID)
	if err != nil {
		return nil, fmt.Errorf("sqlstore: GetPasskeysByUserID: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var pks []*model.PasskeyCredential
	for rows.Next() {
		pk, err := scanPasskey(rows)
		if err != nil {
			return nil, fmt.Errorf("sqlstore: GetPasskeysByUserID scan: %w", err)
		}
		pks = append(pks, pk)
	}
	return pks, rows.Err()
}

func (s *SQLiteStore) GetPasskeyByCredentialID(ctx context.Context, credID []byte) (*model.PasskeyCredential, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, user_id, credential_id, public_key, aaguid, sign_count, transports, name, created_at, last_used_at
		 FROM passkey_credentials WHERE credential_id = ?`, credID)
	pk, err := scanPasskey(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, model.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("sqlstore: GetPasskeyByCredentialID: %w", err)
	}
	return pk, nil
}

func (s *SQLiteStore) UpdatePasskey(ctx context.Context, pk *model.PasskeyCredential) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE passkey_credentials SET sign_count = ?, transports = ?, name = ?, last_used_at = ? WHERE id = ?`,
		pk.SignCount, marshalTransports(pk.Transports), pk.Name, formatTime(pk.LastUsedAt), pk.ID,
	)
	if err != nil {
		return fmt.Errorf("sqlstore: UpdatePasskey: %w", err)
	}
	return nil
}

func (s *SQLiteStore) DeletePasskey(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM passkey_credentials WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("sqlstore: DeletePasskey: %w", err)
	}
	return nil
}

func (s *SQLiteStore) CountPasskeysByUserID(ctx context.Context, userID string) (int, error) {
	var n int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM passkey_credentials WHERE user_id = ?`, userID,
	).Scan(&n); err != nil {
		return 0, fmt.Errorf("sqlstore: CountPasskeysByUserID: %w", err)
	}
	return n, nil
}

// ─── Sessions ───────────────────────────────────────────────────────────────

func (s *SQLiteStore) CreateSession(ctx context.Context, sess *model.Session) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO sessions (id, user_id, created_at, expires_at, last_used_at)
		 VALUES (?, ?, ?, ?, ?)`,
		sess.ID, sess.UserID,
		formatTime(sess.CreatedAt), formatTime(sess.ExpiresAt), formatTime(sess.LastUsedAt),
	)
	if err != nil {
		return fmt.Errorf("sqlstore: CreateSession: %w", err)
	}
	return nil
}

func scanSession(row interface{ Scan(...any) error }) (*model.Session, error) {
	var (
		sess       model.Session
		createdAt  string
		expiresAt  string
		lastUsedAt string
	)
	if err := row.Scan(&sess.ID, &sess.UserID, &createdAt, &expiresAt, &lastUsedAt); err != nil {
		return nil, err
	}
	var err error
	if sess.CreatedAt, err = scanTime(createdAt); err != nil {
		return nil, fmt.Errorf("sqlstore: parse session created_at: %w", err)
	}
	if sess.ExpiresAt, err = scanTime(expiresAt); err != nil {
		return nil, fmt.Errorf("sqlstore: parse session expires_at: %w", err)
	}
	if sess.LastUsedAt, err = scanTime(lastUsedAt); err != nil {
		return nil, fmt.Errorf("sqlstore: parse session last_used_at: %w", err)
	}
	return &sess, nil
}

func (s *SQLiteStore) GetSession(ctx context.Context, id string) (*model.Session, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, user_id, created_at, expires_at, last_used_at FROM sessions WHERE id = ?`, id)
	sess, err := scanSession(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, model.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("sqlstore: GetSession: %w", err)
	}
	return sess, nil
}

func (s *SQLiteStore) TouchSession(ctx context.Context, id string, lastUsed time.Time) error {
	expiresAt := lastUsed.Add(30 * 24 * time.Hour)
	_, err := s.db.ExecContext(ctx,
		`UPDATE sessions SET last_used_at = ?, expires_at = ? WHERE id = ?`,
		formatTime(lastUsed), formatTime(expiresAt), id,
	)
	if err != nil {
		return fmt.Errorf("sqlstore: TouchSession: %w", err)
	}
	return nil
}

func (s *SQLiteStore) DeleteSession(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("sqlstore: DeleteSession: %w", err)
	}
	return nil
}

func (s *SQLiteStore) DeleteExpiredSessions(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM sessions WHERE expires_at < ?`, formatTime(time.Now()))
	if err != nil {
		return fmt.Errorf("sqlstore: DeleteExpiredSessions: %w", err)
	}
	return nil
}

// ─── Devices ────────────────────────────────────────────────────────────────

func scanDevice(row interface{ Scan(...any) error }) (*model.Device, error) {
	var (
		d              model.Device
		apiKey         sql.NullString
		apiKeyIssuedAt sql.NullString
		lastSeenAt     sql.NullString
		lastHeaders    sql.NullString
		createdAt      string
	)
	if err := row.Scan(
		&d.ID, &d.UserID, &d.MACAddress,
		&apiKey, &apiKeyIssuedAt,
		&d.Name, &d.RefreshRate, &d.CanvasConfig, &d.Timezone,
		&createdAt, &lastSeenAt, &d.PlaylistPosition, &lastHeaders,
	); err != nil {
		return nil, err
	}
	if apiKey.Valid {
		d.APIKey = apiKey.String
	}
	d.APIKeyIssuedAt = nullStrTime(apiKeyIssuedAt)
	d.LastSeenAt = nullStrTime(lastSeenAt)
	if lastHeaders.Valid {
		d.LastHeaders = lastHeaders.String
	}

	var err error
	if d.CreatedAt, err = scanTime(createdAt); err != nil {
		return nil, fmt.Errorf("sqlstore: parse device created_at: %w", err)
	}
	return &d, nil
}

func (s *SQLiteStore) CreateDevice(ctx context.Context, d *model.Device) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO devices
		 (id, user_id, mac_address, api_key, api_key_issued_at, name, refresh_rate, canvas_config, timezone, created_at, last_seen_at, playlist_position, last_headers)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		d.ID, d.UserID, d.MACAddress,
		sql.NullString{String: d.APIKey, Valid: d.APIKey != ""},
		nullTimeStr(d.APIKeyIssuedAt),
		d.Name, d.RefreshRate, d.CanvasConfig, d.Timezone,
		formatTime(d.CreatedAt),
		nullTimeStr(d.LastSeenAt),
		d.PlaylistPosition,
		sql.NullString{String: d.LastHeaders, Valid: d.LastHeaders != ""},
	)
	if err != nil {
		return fmt.Errorf("sqlstore: CreateDevice: %w", err)
	}
	return nil
}

func (s *SQLiteStore) GetDeviceByID(ctx context.Context, id string) (*model.Device, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, user_id, mac_address, api_key, api_key_issued_at, name, refresh_rate, canvas_config, timezone, created_at, last_seen_at, playlist_position, last_headers
		 FROM devices WHERE id = ?`, id)
	d, err := scanDevice(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, model.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("sqlstore: GetDeviceByID: %w", err)
	}
	return d, nil
}

func (s *SQLiteStore) GetDeviceByMAC(ctx context.Context, mac string) (*model.Device, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, user_id, mac_address, api_key, api_key_issued_at, name, refresh_rate, canvas_config, timezone, created_at, last_seen_at, playlist_position, last_headers
		 FROM devices WHERE mac_address = ?`, mac)
	d, err := scanDevice(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, model.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("sqlstore: GetDeviceByMAC: %w", err)
	}
	return d, nil
}

func (s *SQLiteStore) GetDeviceByAPIKey(ctx context.Context, apiKey string) (*model.Device, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, user_id, mac_address, api_key, api_key_issued_at, name, refresh_rate, canvas_config, timezone, created_at, last_seen_at, playlist_position, last_headers
		 FROM devices WHERE api_key = ?`, apiKey)
	d, err := scanDevice(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, model.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("sqlstore: GetDeviceByAPIKey: %w", err)
	}
	return d, nil
}

func (s *SQLiteStore) ListDevicesByUser(ctx context.Context, userID string) ([]*model.Device, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, user_id, mac_address, api_key, api_key_issued_at, name, refresh_rate, canvas_config, timezone, created_at, last_seen_at, playlist_position, last_headers
		 FROM devices WHERE user_id = ? ORDER BY created_at`, userID)
	if err != nil {
		return nil, fmt.Errorf("sqlstore: ListDevicesByUser: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var devices []*model.Device
	for rows.Next() {
		d, err := scanDevice(rows)
		if err != nil {
			return nil, fmt.Errorf("sqlstore: ListDevicesByUser scan: %w", err)
		}
		devices = append(devices, d)
	}
	return devices, rows.Err()
}

func (s *SQLiteStore) UpdateDevice(ctx context.Context, d *model.Device) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE devices SET user_id = ?, mac_address = ?, api_key = ?, api_key_issued_at = ?,
		 name = ?, refresh_rate = ?, canvas_config = ?, timezone = ?,
		 last_seen_at = ?, playlist_position = ?, last_headers = ?
		 WHERE id = ?`,
		d.UserID, d.MACAddress,
		sql.NullString{String: d.APIKey, Valid: d.APIKey != ""},
		nullTimeStr(d.APIKeyIssuedAt),
		d.Name, d.RefreshRate, d.CanvasConfig, d.Timezone,
		nullTimeStr(d.LastSeenAt),
		d.PlaylistPosition,
		sql.NullString{String: d.LastHeaders, Valid: d.LastHeaders != ""},
		d.ID,
	)
	if err != nil {
		return fmt.Errorf("sqlstore: UpdateDevice: %w", err)
	}
	return nil
}

func (s *SQLiteStore) DeleteDevice(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM devices WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("sqlstore: DeleteDevice: %w", err)
	}
	return nil
}

func (s *SQLiteStore) IssueDeviceAPIKey(ctx context.Context, deviceID, apiKey string, issuedAt time.Time) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE devices SET api_key = ?, api_key_issued_at = ? WHERE id = ?`,
		apiKey, formatTime(issuedAt), deviceID,
	)
	if err != nil {
		return fmt.Errorf("sqlstore: IssueDeviceAPIKey: %w", err)
	}
	return nil
}

func (s *SQLiteStore) RevokeDeviceAPIKey(ctx context.Context, deviceID string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE devices SET api_key = NULL, api_key_issued_at = NULL WHERE id = ?`, deviceID,
	)
	if err != nil {
		return fmt.Errorf("sqlstore: RevokeDeviceAPIKey: %w", err)
	}
	return nil
}

func (s *SQLiteStore) UpdateDeviceLastSeen(ctx context.Context, deviceID string, seenAt time.Time, headersJSON string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE devices SET last_seen_at = ?, last_headers = ? WHERE id = ?`,
		formatTime(seenAt), headersJSON, deviceID,
	)
	if err != nil {
		return fmt.Errorf("sqlstore: UpdateDeviceLastSeen: %w", err)
	}
	return nil
}

func (s *SQLiteStore) ListAllDevices(ctx context.Context) ([]*model.Device, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, user_id, mac_address, api_key, api_key_issued_at, name, refresh_rate, canvas_config, timezone, created_at, last_seen_at, playlist_position, last_headers
		 FROM devices ORDER BY created_at`)
	if err != nil {
		return nil, fmt.Errorf("sqlstore: ListAllDevices: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var devices []*model.Device
	for rows.Next() {
		d, err := scanDevice(rows)
		if err != nil {
			return nil, fmt.Errorf("sqlstore: ListAllDevices scan: %w", err)
		}
		devices = append(devices, d)
	}
	return devices, rows.Err()
}

func (s *SQLiteStore) SetPlaylistPosition(ctx context.Context, deviceID string, position int) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE devices SET playlist_position = ? WHERE id = ?`, position, deviceID)
	if err != nil {
		return fmt.Errorf("sqlstore: SetPlaylistPosition: %w", err)
	}
	return nil
}

func (s *SQLiteStore) AdvancePlaylistPosition(ctx context.Context, deviceID string, length int) (int, error) {
	_, err := s.db.ExecContext(ctx,
		`UPDATE devices SET playlist_position = (playlist_position + 1) % ? WHERE id = ?`,
		length, deviceID,
	)
	if err != nil {
		return 0, fmt.Errorf("sqlstore: AdvancePlaylistPosition: %w", err)
	}

	var pos int
	if err = s.db.QueryRowContext(ctx,
		`SELECT playlist_position FROM devices WHERE id = ?`, deviceID,
	).Scan(&pos); err != nil {
		return 0, fmt.Errorf("sqlstore: AdvancePlaylistPosition read: %w", err)
	}
	return pos, nil
}

// ─── Playlist items ──────────────────────────────────────────────────────────

func scanPlaylistItem(row interface{ Scan(...any) error }) (*model.PlaylistItem, error) {
	var item model.PlaylistItem
	if err := row.Scan(&item.ID, &item.DeviceID, &item.UserPluginID, &item.Position, &item.Dwell); err != nil {
		return nil, err
	}
	return &item, nil
}

func (s *SQLiteStore) ListPlaylistItems(ctx context.Context, deviceID string) ([]*model.PlaylistItem, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, device_id, user_plugin_id, position, dwell
		 FROM playlist_items WHERE device_id = ? ORDER BY position`, deviceID)
	if err != nil {
		return nil, fmt.Errorf("sqlstore: ListPlaylistItems: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var items []*model.PlaylistItem
	for rows.Next() {
		item, err := scanPlaylistItem(rows)
		if err != nil {
			return nil, fmt.Errorf("sqlstore: ListPlaylistItems scan: %w", err)
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *SQLiteStore) AddPlaylistItem(ctx context.Context, item *model.PlaylistItem) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO playlist_items (id, device_id, user_plugin_id, position, dwell)
		 VALUES (?, ?, ?, ?, ?)`,
		item.ID, item.DeviceID, item.UserPluginID, item.Position, item.Dwell,
	)
	if err != nil {
		return fmt.Errorf("sqlstore: AddPlaylistItem: %w", err)
	}
	return nil
}

func (s *SQLiteStore) UpdatePlaylistItem(ctx context.Context, item *model.PlaylistItem) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE playlist_items SET device_id = ?, user_plugin_id = ?, position = ?, dwell = ? WHERE id = ?`,
		item.DeviceID, item.UserPluginID, item.Position, item.Dwell, item.ID,
	)
	if err != nil {
		return fmt.Errorf("sqlstore: UpdatePlaylistItem: %w", err)
	}
	return nil
}

func (s *SQLiteStore) RemovePlaylistItem(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM playlist_items WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("sqlstore: RemovePlaylistItem: %w", err)
	}
	return nil
}

func (s *SQLiteStore) ReorderPlaylist(ctx context.Context, deviceID string, orderedIDs []string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("sqlstore: ReorderPlaylist begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	for i, id := range orderedIDs {
		if _, err = tx.ExecContext(ctx,
			`UPDATE playlist_items SET position = ? WHERE id = ? AND device_id = ?`,
			i, id, deviceID,
		); err != nil {
			return fmt.Errorf("sqlstore: ReorderPlaylist update: %w", err)
		}
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("sqlstore: ReorderPlaylist commit: %w", err)
	}
	return nil
}

// ─── Library plugins ─────────────────────────────────────────────────────────

func scanLibraryPlugin(row interface{ Scan(...any) error }) (*model.LibraryPlugin, error) {
	var (
		p         model.LibraryPlugin
		isActive  int
		createdAt string
		updatedAt string
	)
	if err := row.Scan(
		&p.ID, &p.Slug, &p.Name, &p.Description, &p.PluginType,
		&p.ConfigSchema, &p.Template, &p.Script, &p.Thumbnail,
		&isActive, &p.SortOrder, &createdAt, &updatedAt,
	); err != nil {
		return nil, err
	}
	p.IsActive = isActive != 0
	var err error
	if p.CreatedAt, err = scanTime(createdAt); err != nil {
		return nil, fmt.Errorf("sqlstore: parse library_plugin created_at: %w", err)
	}
	if p.UpdatedAt, err = scanTime(updatedAt); err != nil {
		return nil, fmt.Errorf("sqlstore: parse library_plugin updated_at: %w", err)
	}
	return &p, nil
}

const libraryPluginColumns = `id, slug, name, description, plugin_type, config_schema, template, script, thumbnail, is_active, sort_order, created_at, updated_at`

func (s *SQLiteStore) ListLibraryPlugins(ctx context.Context, activeOnly bool) ([]*model.LibraryPlugin, error) {
	query := `SELECT ` + libraryPluginColumns + ` FROM library_plugins`
	if activeOnly {
		query += ` WHERE is_active = 1`
	}
	query += ` ORDER BY sort_order, name`

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("sqlstore: ListLibraryPlugins: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var plugins []*model.LibraryPlugin
	for rows.Next() {
		p, err := scanLibraryPlugin(rows)
		if err != nil {
			return nil, fmt.Errorf("sqlstore: ListLibraryPlugins scan: %w", err)
		}
		plugins = append(plugins, p)
	}
	return plugins, rows.Err()
}

func (s *SQLiteStore) GetLibraryPluginBySlug(ctx context.Context, slug string) (*model.LibraryPlugin, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+libraryPluginColumns+` FROM library_plugins WHERE slug = ?`, slug)
	p, err := scanLibraryPlugin(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, model.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("sqlstore: GetLibraryPluginBySlug: %w", err)
	}
	return p, nil
}

func (s *SQLiteStore) GetLibraryPluginByID(ctx context.Context, id string) (*model.LibraryPlugin, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+libraryPluginColumns+` FROM library_plugins WHERE id = ?`, id)
	p, err := scanLibraryPlugin(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, model.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("sqlstore: GetLibraryPluginByID: %w", err)
	}
	return p, nil
}

func (s *SQLiteStore) CreateLibraryPlugin(ctx context.Context, p *model.LibraryPlugin) error {
	var isActive int
	if p.IsActive {
		isActive = 1
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO library_plugins (`+libraryPluginColumns+`)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.ID, p.Slug, p.Name, p.Description, string(p.PluginType),
		p.ConfigSchema, p.Template, p.Script, p.Thumbnail,
		isActive, p.SortOrder, formatTime(p.CreatedAt), formatTime(p.UpdatedAt),
	)
	if err != nil {
		return fmt.Errorf("sqlstore: CreateLibraryPlugin: %w", err)
	}
	return nil
}

func (s *SQLiteStore) UpdateLibraryPlugin(ctx context.Context, p *model.LibraryPlugin) error {
	var isActive int
	if p.IsActive {
		isActive = 1
	}
	_, err := s.db.ExecContext(ctx,
		`UPDATE library_plugins SET slug = ?, name = ?, description = ?, plugin_type = ?,
		 config_schema = ?, template = ?, script = ?, thumbnail = ?,
		 is_active = ?, sort_order = ?, updated_at = ? WHERE id = ?`,
		p.Slug, p.Name, p.Description, string(p.PluginType),
		p.ConfigSchema, p.Template, p.Script, p.Thumbnail,
		isActive, p.SortOrder, formatTime(p.UpdatedAt), p.ID,
	)
	if err != nil {
		return fmt.Errorf("sqlstore: UpdateLibraryPlugin: %w", err)
	}
	return nil
}

// ─── User plugins ────────────────────────────────────────────────────────────

func scanUserPlugin(row interface{ Scan(...any) error }) (*model.UserPlugin, error) {
	var (
		p               model.UserPlugin
		libraryPluginID sql.NullString
		lastRenderedAt  sql.NullString
		createdAt       string
		updatedAt       string
	)
	if err := row.Scan(
		&p.ID, &p.UserID, &libraryPluginID, &p.ExternalURL,
		&p.Name, &p.PluginType, &p.Config, &p.Template, &p.Script, &p.Refresh,
		&createdAt, &updatedAt, &lastRenderedAt,
	); err != nil {
		return nil, err
	}
	if libraryPluginID.Valid {
		p.LibraryPluginID = libraryPluginID.String
	}
	p.LastRenderedAt = nullStrTime(lastRenderedAt)

	var err error
	if p.CreatedAt, err = scanTime(createdAt); err != nil {
		return nil, fmt.Errorf("sqlstore: parse user_plugin created_at: %w", err)
	}
	if p.UpdatedAt, err = scanTime(updatedAt); err != nil {
		return nil, fmt.Errorf("sqlstore: parse user_plugin updated_at: %w", err)
	}
	return &p, nil
}

const userPluginColumns = `id, user_id, library_plugin_id, external_url, name, plugin_type, config, template, script, refresh, created_at, updated_at, last_rendered_at`

func (s *SQLiteStore) CreateUserPlugin(ctx context.Context, p *model.UserPlugin) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO user_plugins (`+userPluginColumns+`)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.ID,
		p.UserID,
		sql.NullString{String: p.LibraryPluginID, Valid: p.LibraryPluginID != ""},
		p.ExternalURL,
		p.Name,
		string(p.PluginType),
		p.Config,
		p.Template,
		p.Script,
		p.Refresh,
		formatTime(p.CreatedAt),
		formatTime(p.UpdatedAt),
		nullTimeStr(p.LastRenderedAt),
	)
	if err != nil {
		return fmt.Errorf("sqlstore: CreateUserPlugin: %w", err)
	}
	return nil
}

func (s *SQLiteStore) GetUserPluginByID(ctx context.Context, id string) (*model.UserPlugin, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+userPluginColumns+` FROM user_plugins WHERE id = ?`, id)
	p, err := scanUserPlugin(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, model.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("sqlstore: GetUserPluginByID: %w", err)
	}
	return p, nil
}

func (s *SQLiteStore) ListUserPlugins(ctx context.Context, userID string) ([]*model.UserPlugin, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+userPluginColumns+` FROM user_plugins WHERE user_id = ? ORDER BY created_at`, userID)
	if err != nil {
		return nil, fmt.Errorf("sqlstore: ListUserPlugins: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var plugins []*model.UserPlugin
	for rows.Next() {
		p, err := scanUserPlugin(rows)
		if err != nil {
			return nil, fmt.Errorf("sqlstore: ListUserPlugins scan: %w", err)
		}
		plugins = append(plugins, p)
	}
	return plugins, rows.Err()
}

func (s *SQLiteStore) UpdateUserPlugin(ctx context.Context, p *model.UserPlugin) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE user_plugins SET library_plugin_id = ?, external_url = ?, name = ?, plugin_type = ?,
		 config = ?, template = ?, script = ?, refresh = ?, updated_at = ?, last_rendered_at = ?
		 WHERE id = ?`,
		sql.NullString{String: p.LibraryPluginID, Valid: p.LibraryPluginID != ""},
		p.ExternalURL, p.Name, string(p.PluginType),
		p.Config, p.Template, p.Script, p.Refresh,
		formatTime(p.UpdatedAt),
		nullTimeStr(p.LastRenderedAt),
		p.ID,
	)
	if err != nil {
		return fmt.Errorf("sqlstore: UpdateUserPlugin: %w", err)
	}
	return nil
}

func (s *SQLiteStore) DeleteUserPlugin(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM user_plugins WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("sqlstore: DeleteUserPlugin: %w", err)
	}
	return nil
}

// ─── Resource cache ──────────────────────────────────────────────────────────

func scanCachedResource(row interface{ Scan(...any) error }) (*model.CachedResource, error) {
	var (
		r         model.CachedResource
		fetchedAt string
		expiresAt string
	)
	if err := row.Scan(
		&r.Key, &r.URL, &r.UserID, &r.Data, &r.ContentType, &fetchedAt, &expiresAt,
	); err != nil {
		return nil, err
	}
	var err error
	if r.FetchedAt, err = scanTime(fetchedAt); err != nil {
		return nil, fmt.Errorf("sqlstore: parse resource_cache fetched_at: %w", err)
	}
	if r.ExpiresAt, err = scanTime(expiresAt); err != nil {
		return nil, fmt.Errorf("sqlstore: parse resource_cache expires_at: %w", err)
	}
	return &r, nil
}

func (s *SQLiteStore) GetCachedResource(ctx context.Context, key string) (*model.CachedResource, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT key, url, user_id, data, content_type, fetched_at, expires_at
		 FROM resource_cache WHERE key = ?`, key)
	r, err := scanCachedResource(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, model.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("sqlstore: GetCachedResource: %w", err)
	}
	return r, nil
}

func (s *SQLiteStore) SetCachedResource(ctx context.Context, r *model.CachedResource) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO resource_cache (key, url, user_id, data, content_type, fetched_at, expires_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(key) DO UPDATE SET
		   url = excluded.url,
		   user_id = excluded.user_id,
		   data = excluded.data,
		   content_type = excluded.content_type,
		   fetched_at = excluded.fetched_at,
		   expires_at = excluded.expires_at`,
		r.Key, r.URL, r.UserID, r.Data, r.ContentType,
		formatTime(r.FetchedAt), formatTime(r.ExpiresAt),
	)
	if err != nil {
		return fmt.Errorf("sqlstore: SetCachedResource: %w", err)
	}
	return nil
}

func (s *SQLiteStore) EvictExpiredResources(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM resource_cache WHERE expires_at < ?`, formatTime(time.Now()))
	if err != nil {
		return fmt.Errorf("sqlstore: EvictExpiredResources: %w", err)
	}
	return nil
}

// ─── Device logs ─────────────────────────────────────────────────────────────

func (s *SQLiteStore) InsertDeviceLog(ctx context.Context, entry *model.DeviceLogEntry) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO device_logs
		 (id, device_id, logged_at, message, wifi_status, wake_reason, firmware_version,
		  battery_voltage, wifi_signal, free_heap_size, special_function)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		entry.ID, entry.DeviceID, formatTime(entry.LoggedAt),
		entry.Message, entry.WiFiStatus, entry.WakeReason, entry.FirmwareVersion,
		entry.BatteryVoltage, entry.WiFiSignal, entry.FreeHeapSize, entry.SpecialFunction,
	)
	if err != nil {
		return fmt.Errorf("sqlstore: InsertDeviceLog: %w", err)
	}
	return nil
}
