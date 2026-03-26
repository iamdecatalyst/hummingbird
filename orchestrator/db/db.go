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
	encryptionKey []byte // 32 bytes (AES-256)
}

type User struct {
	WalletID  string
	CreatedAt time.Time
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
	// wallet_id = the Signet wallet ID — stable identity, no passwords needed.
	_, err := d.sql.Exec(`
		CREATE TABLE IF NOT EXISTS users (
			wallet_id     TEXT PRIMARY KEY,
			signet_key    BYTEA NOT NULL,
			signet_secret BYTEA NOT NULL,
			created_at    TIMESTAMPTZ DEFAULT NOW()
		)
	`)
	return err
}

// Upsert saves (or updates) a user's encrypted Signet credentials keyed by wallet ID.
func (d *DB) Upsert(walletID, apiKey, apiSecret string) error {
	encKey, err := d.encrypt([]byte(apiKey))
	if err != nil {
		return err
	}
	encSecret, err := d.encrypt([]byte(apiSecret))
	if err != nil {
		return err
	}
	_, err = d.sql.Exec(`
		INSERT INTO users (wallet_id, signet_key, signet_secret)
		VALUES ($1, $2, $3)
		ON CONFLICT (wallet_id) DO UPDATE
			SET signet_key=$2, signet_secret=$3
	`, walletID, encKey, encSecret)
	return err
}

// GetCredentials decrypts and returns the Signet credentials for a wallet ID.
func (d *DB) GetCredentials(walletID string) (apiKey, apiSecret string, err error) {
	var encKey, encSecret []byte
	err = d.sql.QueryRow(
		`SELECT signet_key, signet_secret FROM users WHERE wallet_id=$1`, walletID,
	).Scan(&encKey, &encSecret)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", fmt.Errorf("wallet not found")
	}
	if err != nil {
		return "", "", err
	}
	rawKey, err := d.decrypt(encKey)
	if err != nil {
		return "", "", err
	}
	rawSecret, err := d.decrypt(encSecret)
	if err != nil {
		return "", "", err
	}
	return string(rawKey), string(rawSecret), nil
}

// GetUser returns basic info for a wallet ID.
func (d *DB) GetUser(walletID string) (*User, error) {
	var u User
	err := d.sql.QueryRow(
		`SELECT wallet_id, created_at FROM users WHERE wallet_id=$1`, walletID,
	).Scan(&u.WalletID, &u.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return &u, err
}

// AllWalletIDs returns all wallet IDs (used at startup to resume bots).
func (d *DB) AllWalletIDs() ([]string, error) {
	rows, err := d.sql.Query(`SELECT wallet_id FROM users`)
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

// encrypt uses AES-256-GCM. Nonce is prepended to ciphertext.
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
