package db

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"time"

	_ "github.com/lib/pq"
)

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
	_, err := d.sql.Exec(`ALTER TABLE hb_users ADD COLUMN IF NOT EXISTS telegram_chat_id TEXT NOT NULL DEFAULT ''`)
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
