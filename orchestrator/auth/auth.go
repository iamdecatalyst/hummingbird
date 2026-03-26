package auth

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const tokenTTL = 30 * 24 * time.Hour // 30 days

type Claims struct {
	WalletID string `json:"wid"`
	jwt.RegisteredClaims
}

// IssueToken creates a signed JWT for the given wallet ID.
func IssueToken(walletID, secret string) (string, error) {
	claims := Claims{
		WalletID: walletID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(tokenTTL)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
}

// ParseToken validates the JWT and returns the wallet ID.
func ParseToken(tokenStr, secret string) (string, error) {
	tok, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return []byte(secret), nil
	})
	if err != nil {
		return "", err
	}
	claims, ok := tok.Claims.(*Claims)
	if !ok || !tok.Valid {
		return "", errors.New("invalid token")
	}
	return claims.WalletID, nil
}

// RandomID generates a random 16-byte hex string (not used for auth, just helpers).
func RandomID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}
