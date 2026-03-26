package client

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// configDir returns ~/.config/hummingbird/
func configDir() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "hummingbird")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "hummingbird")
}

var credentialsFile = filepath.Join(configDir(), "credentials")

// LoadCredentials reads the saved API URL and token from ~/.config/hummingbird/credentials.
// Returns ("", "") if not set.
func LoadCredentials() (apiURL, token string) {
	data, err := os.ReadFile(credentialsFile)
	if err != nil {
		return "", ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "api_url=") {
			apiURL = strings.TrimPrefix(line, "api_url=")
		}
		if strings.HasPrefix(line, "hb_token=") {
			token = strings.TrimPrefix(line, "hb_token=")
		}
	}
	return apiURL, token
}

// LoadToken reads only the saved token.
func LoadToken() string {
	_, token := LoadCredentials()
	return token
}

// SaveCredentials writes the API URL and token to ~/.config/hummingbird/credentials.
func SaveCredentials(apiURL, token string) error {
	dir := configDir()
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("creating config dir: %w", err)
	}
	content := fmt.Sprintf("api_url=%s\nhb_token=%s\n",
		strings.TrimSpace(apiURL),
		strings.TrimSpace(token),
	)
	return os.WriteFile(credentialsFile, []byte(content), 0600)
}

// RemoveCredentials deletes the saved credentials file.
func RemoveCredentials() error {
	err := os.Remove(credentialsFile)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// CredentialsPath returns the path to the credentials file (for display).
func CredentialsPath() string {
	return credentialsFile
}
