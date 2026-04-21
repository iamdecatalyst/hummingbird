package db

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"time"

	_ "github.com/lib/pq"

	"github.com/iamdecatalyst/hummingbird/orchestrator/eventlog"
	"github.com/iamdecatalyst/hummingbird/orchestrator/models"
)

// UserConfig holds per-user trading settings.
type UserConfig struct {
	SniperEnabled   bool    `json:"sniper_enabled"`
	ScalperEnabled  bool    `json:"scalper_enabled"`
	SwingEnabled    bool    `json:"swing_enabled"`
	MaxPositionSOL  float64 `json:"max_position_sol"`
	MaxPositions    int     `json:"max_positions"`
	StopLossPercent float64 `json:"stop_loss_percent"` // 0.25 = 25%
	DailyLossLimit  float64 `json:"daily_loss_limit"`  // 0.30 = 30%
	TakeProfit1x    float64 `json:"take_profit_1x"`    // price multiple, e.g. 2.0
	TakeProfit2x    float64 `json:"take_profit_2x"`
	TakeProfit3x    float64 `json:"take_profit_3x"`
	TimeoutMinutes  int     `json:"timeout_minutes"`
	MinBalanceSOL   float64 `json:"min_balance_sol"`
}

// DefaultUserConfig returns sane defaults for new users.
func DefaultUserConfig() *UserConfig {
	return &UserConfig{
		SniperEnabled:   true,
		ScalperEnabled:  true,
		SwingEnabled:    true,
		MaxPositionSOL:  0.10,
		MaxPositions:    5,
		StopLossPercent: 0.25,
		DailyLossLimit:  0.30,
		TakeProfit1x:    2.0,
		TakeProfit2x:    5.0,
		TakeProfit3x:    10.0,
		TimeoutMinutes:  8,
		MinBalanceSOL:   0.0,
	}
}

type DB struct {
	sql           *sql.DB
	encryptionKey []byte
}

type User struct {
	NexusUserID      string
	Username         string
	FirstName        string
	LastName         string
	Email            string
	Avatar           string
	HasSignet        bool // true once Signet key has been configured
	SignetKeyPrefix  string
	WalletID         string
	MainWalletID     string
	TelegramChatID   string
	CreatedAt        time.Time
}

func New(databaseURL, encryptionKeyHex string) (*DB, error) {
	if len(encryptionKeyHex) != 64 {
		return nil, fmt.Errorf("ENCRYPTION_KEY must be 64 hex chars (32 bytes); got %d", len(encryptionKeyHex))
	}
	key, err := hex.DecodeString(encryptionKeyHex)
	if err != nil {
		return nil, fmt.Errorf("invalid ENCRYPTION_KEY: %w", err)
	}
	sqlDB, err := sql.Open("postgres", databaseURL)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	if err := sqlDB.Ping(); err != nil {
		return nil, fmt.Errorf("ping db: %w", err)
	}
	d := &DB{sql: sqlDB, encryptionKey: key}
	return d, d.migrate()
}

func (d *DB) migrate() error {
	if _, err := d.sql.Exec(`
		CREATE TABLE IF NOT EXISTS hb_users (
			nexus_user_id TEXT PRIMARY KEY,
			username      TEXT NOT NULL DEFAULT '',
			first_name    TEXT NOT NULL DEFAULT '',
			last_name     TEXT NOT NULL DEFAULT '',
			email         TEXT NOT NULL DEFAULT '',
			avatar        TEXT NOT NULL DEFAULT '',
			signet_key    BYTEA,
			signet_secret BYTEA,
			wallet_id     TEXT NOT NULL DEFAULT '',
			created_at    TIMESTAMPTZ DEFAULT NOW()
		)
	`); err != nil {
		return err
	}
	if _, err := d.sql.Exec(`ALTER TABLE hb_users ADD COLUMN IF NOT EXISTS username TEXT NOT NULL DEFAULT ''`); err != nil {
		return err
	}
	if _, err := d.sql.Exec(`ALTER TABLE hb_users ADD COLUMN IF NOT EXISTS signet_key_prefix TEXT NOT NULL DEFAULT ''`); err != nil {
		return err
	}
	if _, err := d.sql.Exec(`ALTER TABLE hb_users ADD COLUMN IF NOT EXISTS main_wallet_id TEXT NOT NULL DEFAULT ''`); err != nil {
		return err
	}
	if _, err := d.sql.Exec(`ALTER TABLE hb_users ADD COLUMN IF NOT EXISTS telegram_chat_id TEXT NOT NULL DEFAULT ''`); err != nil {
		return err
	}
	// Per-user trading config (JSON blob — easy to extend without further migrations)
	if _, err := d.sql.Exec(`
		CREATE TABLE IF NOT EXISTS hb_user_configs (
			nexus_user_id TEXT PRIMARY KEY,
			config_json   JSONB NOT NULL DEFAULT '{}'
		)
	`); err != nil {
		return err
	}
	// Per-user event log (persisted across restarts)
	if _, err := d.sql.Exec(`
		CREATE TABLE IF NOT EXISTS hb_events (
			id            BIGSERIAL PRIMARY KEY,
			nexus_user_id TEXT NOT NULL,
			time          TIMESTAMPTZ NOT NULL,
			type          TEXT NOT NULL DEFAULT '',
			token         TEXT NOT NULL DEFAULT '',
			amount_sol    DOUBLE PRECISION NOT NULL DEFAULT 0,
			pnl_sol       DOUBLE PRECISION NOT NULL DEFAULT 0,
			pnl_pct       DOUBLE PRECISION NOT NULL DEFAULT 0,
			reason        TEXT NOT NULL DEFAULT '',
			message       TEXT NOT NULL DEFAULT ''
		)
	`); err != nil {
		return err
	}
	if _, err := d.sql.Exec(`CREATE INDEX IF NOT EXISTS hb_events_user_time ON hb_events (nexus_user_id, time DESC)`); err != nil {
		return err
	}
	if _, err := d.sql.Exec(`
		CREATE TABLE IF NOT EXISTS hb_positions (
			id            TEXT PRIMARY KEY,
			nexus_user_id TEXT NOT NULL REFERENCES hb_users(nexus_user_id),
			mint          TEXT NOT NULL,
			dev_wallet    TEXT NOT NULL DEFAULT '',
			wallet_id     TEXT NOT NULL DEFAULT '',
			entry_price   DOUBLE PRECISION NOT NULL DEFAULT 0,
			entry_sol     DOUBLE PRECISION NOT NULL DEFAULT 0,
			token_balance DOUBLE PRECISION NOT NULL DEFAULT 0,
			score         INTEGER NOT NULL DEFAULT 0,
			decision      TEXT NOT NULL DEFAULT '',
			status        TEXT NOT NULL DEFAULT 'open',
			opened_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			peak_price    DOUBLE PRECISION NOT NULL DEFAULT 0,
			take_profit_level INTEGER NOT NULL DEFAULT 0,
			closed_at     TIMESTAMPTZ,
			exit_price    DOUBLE PRECISION,
			exit_sol      DOUBLE PRECISION,
			exit_reason   TEXT,
			pnl_sol       DOUBLE PRECISION,
			pnl_percent   DOUBLE PRECISION
		)
	`); err != nil {
		return err
	}
	// Idempotent migration for already-deployed installs predating the column.
	if _, err := d.sql.Exec(`ALTER TABLE hb_positions ADD COLUMN IF NOT EXISTS take_profit_level INTEGER NOT NULL DEFAULT 0`); err != nil {
		return err
	}
	_, err := d.sql.Exec(`CREATE INDEX IF NOT EXISTS idx_positions_user_status ON hb_positions (nexus_user_id, status)`)
	return err
}

// SavePosition inserts a new open position into the DB.
func (d *DB) SavePosition(nexusUserID string, pos *models.Position) error {
	_, err := d.sql.Exec(`
		INSERT INTO hb_positions
			(id, nexus_user_id, mint, dev_wallet, wallet_id, entry_price, entry_sol, token_balance, score, decision, status, opened_at, peak_price, take_profit_level)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,'open',$11,$12,$13)
		ON CONFLICT (id) DO NOTHING
	`, pos.ID, nexusUserID, pos.Mint, pos.DevWallet, pos.WalletID,
		pos.EntryPriceSOL, pos.EntryAmountSOL, pos.TokenBalance,
		pos.Score, pos.Decision, pos.OpenedAt, pos.PeakPriceSOL, pos.TakeProfitLevel)
	return err
}

// UpdatePositionProgress persists peak price + TP level — called when monitor
// advances a TP threshold so that a restart doesn't re-fire TP1/TP2 (losing
// 40% of remaining position per crash).
func (d *DB) UpdatePositionProgress(nexusUserID, posID string, peakPrice float64, tpLevel int) error {
	_, err := d.sql.Exec(`
		UPDATE hb_positions SET peak_price=$1, take_profit_level=$2
		WHERE id=$3 AND nexus_user_id=$4 AND status='open'
	`, peakPrice, tpLevel, posID, nexusUserID)
	return err
}

// ClosePosition marks a position as closed with final P&L.
func (d *DB) ClosePosition(nexusUserID string, closed *models.ClosedPosition) error {
	_, err := d.sql.Exec(`
		UPDATE hb_positions SET
			status='closed', closed_at=$1, exit_price=$2, exit_sol=$3,
			exit_reason=$4, pnl_sol=$5, pnl_percent=$6
		WHERE id=$7 AND nexus_user_id=$8
	`, closed.ClosedAt, closed.ExitPriceSOL, closed.ExitAmountSOL,
		string(closed.Reason), closed.PnLSOL, closed.PnLPercent,
		closed.ID, nexusUserID)
	return err
}

// OpenPositionsByUser returns all open positions for a user (for restart recovery).
func (d *DB) OpenPositionsByUser(nexusUserID string) ([]*models.Position, error) {
	rows, err := d.sql.Query(`
		SELECT id, mint, dev_wallet, wallet_id, entry_price, entry_sol, token_balance, score, decision, opened_at, peak_price, take_profit_level
		FROM hb_positions WHERE nexus_user_id=$1 AND status='open'
		ORDER BY opened_at ASC
	`, nexusUserID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var positions []*models.Position
	for rows.Next() {
		var p models.Position
		if err := rows.Scan(
			&p.ID, &p.Mint, &p.DevWallet, &p.WalletID,
			&p.EntryPriceSOL, &p.EntryAmountSOL, &p.TokenBalance,
			&p.Score, &p.Decision, &p.OpenedAt, &p.PeakPriceSOL, &p.TakeProfitLevel,
		); err == nil {
			positions = append(positions, &p)
		}
	}
	return positions, rows.Err()
}

// ClosedPositionsByUser returns the last N closed positions for a user (newest first).
func (d *DB) ClosedPositionsByUser(nexusUserID string, limit int) ([]*models.ClosedPosition, error) {
	rows, err := d.sql.Query(`
		SELECT id, mint, dev_wallet, wallet_id, entry_price, entry_sol, token_balance, score, decision, opened_at, peak_price,
		       closed_at, exit_price, exit_sol, exit_reason, pnl_sol, pnl_percent
		FROM hb_positions
		WHERE nexus_user_id=$1 AND status='closed'
		ORDER BY closed_at DESC LIMIT $2
	`, nexusUserID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*models.ClosedPosition
	for rows.Next() {
		var c models.ClosedPosition
		var closedAt sql.NullTime
		var exitPrice, exitSOL, pnlSOL, pnlPct sql.NullFloat64
		var exitReason sql.NullString
		if scanErr := rows.Scan(
			&c.ID, &c.Mint, &c.DevWallet, &c.WalletID,
			&c.EntryPriceSOL, &c.EntryAmountSOL, &c.TokenBalance,
			&c.Score, &c.Decision, &c.OpenedAt, &c.PeakPriceSOL,
			&closedAt, &exitPrice, &exitSOL, &exitReason, &pnlSOL, &pnlPct,
		); scanErr != nil {
			log.Printf("[db] ClosedPositionsByUser scan error: %v", scanErr)
		} else {
			if closedAt.Valid {
				c.ClosedAt = closedAt.Time
			}
			if exitPrice.Valid {
				c.ExitPriceSOL = exitPrice.Float64
			}
			if exitSOL.Valid {
				c.ExitAmountSOL = exitSOL.Float64
			}
			if exitReason.Valid {
				c.Reason = models.ExitReason(exitReason.String)
			}
			if pnlSOL.Valid {
				c.PnLSOL = pnlSOL.Float64
			}
			if pnlPct.Valid {
				c.PnLPercent = pnlPct.Float64
			}
			out = append(out, &c)
		}
	}
	return out, rows.Err()
}

// InsertEvent persists a single event for a user. Fire-and-forget safe.
func (d *DB) InsertEvent(nexusUserID string, e eventlog.Event) {
	d.sql.Exec(`
		INSERT INTO hb_events (nexus_user_id, time, type, token, amount_sol, pnl_sol, pnl_pct, reason, message)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, nexusUserID, e.Time, e.Type, e.Token, e.AmtSOL, e.PnLSOL, e.PnLPct, e.Reason, e.Message)
}

// RecentEvents loads the most recent events for a user, returned oldest-first
// so they can be passed directly to Log.Load.
func (d *DB) RecentEvents(nexusUserID string, limit int) ([]eventlog.Event, error) {
	rows, err := d.sql.Query(`
		SELECT time, type, token, amount_sol, pnl_sol, pnl_pct, reason, message
		FROM hb_events WHERE nexus_user_id=$1
		ORDER BY time DESC LIMIT $2
	`, nexusUserID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var events []eventlog.Event
	for rows.Next() {
		var e eventlog.Event
		if err := rows.Scan(&e.Time, &e.Type, &e.Token, &e.AmtSOL, &e.PnLSOL, &e.PnLPct, &e.Reason, &e.Message); err == nil {
			events = append(events, e)
		}
	}
	// reverse to oldest-first for Log.Load
	for i, j := 0, len(events)-1; i < j; i, j = i+1, j-1 {
		events[i], events[j] = events[j], events[i]
	}
	return events, rows.Err()
}

// GetUserConfig loads per-user config, falling back to defaults if not set.
func (d *DB) GetUserConfig(nexusUserID string) (*UserConfig, error) {
	var raw []byte
	err := d.sql.QueryRow(
		`SELECT config_json FROM hb_user_configs WHERE nexus_user_id=$1`, nexusUserID,
	).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return DefaultUserConfig(), nil
	}
	if err != nil {
		return nil, err
	}
	cfg := DefaultUserConfig()
	if err := json.Unmarshal(raw, cfg); err != nil {
		return DefaultUserConfig(), nil
	}
	return cfg, nil
}

// SetUserConfig saves per-user config.
func (d *DB) SetUserConfig(nexusUserID string, cfg *UserConfig) error {
	raw, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	_, err = d.sql.Exec(`
		INSERT INTO hb_user_configs (nexus_user_id, config_json)
		VALUES ($1, $2)
		ON CONFLICT (nexus_user_id) DO UPDATE SET config_json=$2
	`, nexusUserID, raw)
	return err
}

// UpsertProfile creates or updates a user's Nexus profile info.
func (d *DB) UpsertProfile(nexusUserID, username, firstName, lastName, email, avatar string) error {
	_, err := d.sql.Exec(`
		INSERT INTO hb_users (nexus_user_id, username, first_name, last_name, email, avatar)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (nexus_user_id) DO UPDATE
			SET username=$2, first_name=$3, last_name=$4, email=$5, avatar=$6
	`, nexusUserID, username, firstName, lastName, email, avatar)
	return err
}

// GetUser fetches a user by Nexus user ID.
func (d *DB) GetUser(nexusUserID string) (*User, error) {
	var u User
	err := d.sql.QueryRow(`
		SELECT nexus_user_id, COALESCE(username,''), first_name, last_name, email, avatar,
		       (signet_key IS NOT NULL), COALESCE(signet_key_prefix,''), COALESCE(wallet_id,''),
		       COALESCE(main_wallet_id,''), COALESCE(telegram_chat_id,''), created_at
		FROM hb_users WHERE nexus_user_id=$1
	`, nexusUserID).Scan(
		&u.NexusUserID, &u.Username, &u.FirstName, &u.LastName, &u.Email, &u.Avatar,
		&u.HasSignet, &u.SignetKeyPrefix, &u.WalletID, &u.MainWalletID, &u.TelegramChatID, &u.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return &u, err
}

// SetSignetCredentials encrypts and saves the user's Signet API key + secret + wallet ID.
func (d *DB) SetSignetCredentials(nexusUserID, apiKey, apiSecret, walletID string) error {
	encKey, err := d.encrypt([]byte(apiKey))
	if err != nil {
		return err
	}
	encSecret, err := d.encrypt([]byte(apiSecret))
	if err != nil {
		return err
	}
	prefix := apiKey
	if len(prefix) > 16 {
		prefix = prefix[:16] + "…"
	}
	_, err = d.sql.Exec(`
		UPDATE hb_users SET signet_key=$1, signet_secret=$2, wallet_id=$3, signet_key_prefix=$4 WHERE nexus_user_id=$5
	`, encKey, encSecret, walletID, prefix, nexusUserID)
	return err
}

// ClearSignetCredentials removes the user's Signet credentials (does not delete the user row).
func (d *DB) ClearSignetCredentials(nexusUserID string) error {
	_, err := d.sql.Exec(`
		UPDATE hb_users SET signet_key=NULL, signet_secret=NULL, signet_key_prefix='', wallet_id='' WHERE nexus_user_id=$1
	`, nexusUserID)
	return err
}

// SetMainWallet sets the wallet the bot should trade with.
func (d *DB) SetMainWallet(nexusUserID, walletID string) error {
	_, err := d.sql.Exec(`UPDATE hb_users SET main_wallet_id=$1 WHERE nexus_user_id=$2`, walletID, nexusUserID)
	return err
}

// SetTelegramChatID stores the user's Telegram chat ID after deep-link linking.
func (d *DB) SetTelegramChatID(nexusUserID string, chatID string) error {
	_, err := d.sql.Exec(`UPDATE hb_users SET telegram_chat_id=$1 WHERE nexus_user_id=$2`, chatID, nexusUserID)
	return err
}

// AllConfiguredUsersData returns full User records for users with Signet credentials.
func (d *DB) AllConfiguredUsersData() ([]*User, error) {
	rows, err := d.sql.Query(`
		SELECT nexus_user_id, COALESCE(username,''), first_name, last_name, email, avatar,
		       (signet_key IS NOT NULL), COALESCE(signet_key_prefix,''), COALESCE(wallet_id,''),
		       COALESCE(main_wallet_id,''), COALESCE(telegram_chat_id,''), created_at
		FROM hb_users WHERE signet_key IS NOT NULL
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var users []*User
	for rows.Next() {
		var u User
		if err := rows.Scan(
			&u.NexusUserID, &u.Username, &u.FirstName, &u.LastName, &u.Email, &u.Avatar,
			&u.HasSignet, &u.SignetKeyPrefix, &u.WalletID, &u.MainWalletID, &u.TelegramChatID, &u.CreatedAt,
		); err == nil {
			users = append(users, &u)
		}
	}
	return users, rows.Err()
}

// GetSignetCredentials decrypts and returns the user's Signet API key + secret.
func (d *DB) GetSignetCredentials(nexusUserID string) (apiKey, apiSecret string, err error) {
	var encKey, encSecret []byte
	err = d.sql.QueryRow(
		`SELECT signet_key, signet_secret FROM hb_users WHERE nexus_user_id=$1`, nexusUserID,
	).Scan(&encKey, &encSecret)
	if err != nil {
		return "", "", err
	}
	if encKey == nil {
		return "", "", fmt.Errorf("no Signet credentials configured")
	}
	raw, err := d.decrypt(encKey)
	if err != nil {
		return "", "", err
	}
	rawS, err := d.decrypt(encSecret)
	if err != nil {
		return "", "", err
	}
	return string(raw), string(rawS), nil
}

// AllConfiguredUsers returns nexus_user_id for users who have Signet credentials set.
func (d *DB) AllConfiguredUsers() ([]string, error) {
	rows, err := d.sql.Query(`SELECT nexus_user_id FROM hb_users WHERE signet_key IS NOT NULL`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err == nil {
			ids = append(ids, id)
		}
	}
	return ids, rows.Err()
}

func (d *DB) encrypt(plain []byte) ([]byte, error) {
	block, err := aes.NewCipher(d.encryptionKey)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return gcm.Seal(nonce, nonce, plain, nil), nil
}

func (d *DB) decrypt(data []byte) ([]byte, error) {
	block, err := aes.NewCipher(d.encryptionKey)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	ns := gcm.NonceSize()
	if len(data) < ns {
		return nil, fmt.Errorf("ciphertext too short")
	}
	return gcm.Open(nil, data[:ns], data[ns:], nil)
}
