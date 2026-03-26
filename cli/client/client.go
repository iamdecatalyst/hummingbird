package client

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

// Client is the HTTP client for the Hummingbird orchestrator API.
type Client struct {
	BaseURL    string
	Token      string
	httpClient *http.Client
}

// New creates a new Hummingbird API client.
// Returns (client, notLoggedIn) — notLoggedIn is true if no credentials are saved.
func New(baseURL, token string) (*Client, bool) {
	savedURL, savedToken := LoadCredentials()

	if baseURL == "" {
		baseURL = os.Getenv("HUMMINGBIRD_API_URL")
	}
	if baseURL == "" {
		baseURL = savedURL
	}

	if token == "" {
		token = os.Getenv("HUMMINGBIRD_TOKEN")
	}
	if token == "" {
		token = savedToken
	}

	notLoggedIn := baseURL == "" || token == ""

	return &Client{
		BaseURL: baseURL,
		Token:   token,
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}, notLoggedIn
}

// Stats holds the response from GET /stats.
type Stats struct {
	OpenPositions int     `json:"open_positions"`
	TotalTrades   int     `json:"total_trades"`
	Wins          int     `json:"wins"`
	Losses        int     `json:"losses"`
	WinRate       float64 `json:"win_rate"`
	TodayPnL      float64 `json:"today_pnl"`
	TotalPnL      float64 `json:"total_pnl"`
	Paused        bool    `json:"paused"`
	PauseReason   string  `json:"pause_reason"`
	Configured    bool    `json:"configured"`
}

// Position holds an open position from GET /positions.
type Position struct {
	ID             string  `json:"id"`
	Mint           string  `json:"mint"`
	EntryPriceSOL  float64 `json:"entry_price_sol"`
	EntryAmountSOL float64 `json:"entry_amount_sol"`
	TokenBalance   float64 `json:"token_balance"`
	Score          int     `json:"score"`
	OpenedAt       string  `json:"opened_at"`
}

// ClosedPosition holds a closed position from GET /closed.
type ClosedPosition struct {
	Position
	ExitPriceSOL  float64 `json:"exit_price_sol"`
	ExitAmountSOL float64 `json:"exit_amount_sol"`
	PnLSOL        float64 `json:"pnl_sol"`
	PnLPercent    float64 `json:"pnl_percent"`
	Reason        string  `json:"reason"`
	ClosedAt      string  `json:"closed_at"`
}

// LogEntry holds a log event from GET /logs.
type LogEntry struct {
	Time    string  `json:"time"`
	Type    string  `json:"type"`
	Token   string  `json:"token,omitempty"`
	AmtSOL  float64 `json:"amount_sol,omitempty"`
	PnLSOL  float64 `json:"pnl_sol,omitempty"`
	PnLPct  float64 `json:"pnl_pct,omitempty"`
	Reason  string  `json:"reason,omitempty"`
	Message string  `json:"message"`
}

// ModeResponse holds the response from GET /mode.
type ModeResponse struct {
	MultiTenant bool `json:"multi_tenant"`
}

func (c *Client) doPost(path string) error {
	url := c.BaseURL + path

	req, err := http.NewRequest("POST", url, nil)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

// Stop pauses the trading bot.
func (c *Client) Stop() error { return c.doPost("/stop") }

// Resume unpauses the trading bot.
func (c *Client) Resume() error { return c.doPost("/resume") }

// Start activates the bot (first launch).
func (c *Client) Start() error { return c.doPost("/start") }

func (c *Client) doRequest(path string) ([]byte, error) {
	url := c.BaseURL + path

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	return body, nil
}

// GetStats fetches the trading stats.
func (c *Client) GetStats() (*Stats, error) {
	data, err := c.doRequest("/stats")
	if err != nil {
		return nil, err
	}
	var result Stats
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("decoding stats: %w", err)
	}
	return &result, nil
}

// GetPositions fetches open positions.
func (c *Client) GetPositions() ([]Position, error) {
	data, err := c.doRequest("/positions")
	if err != nil {
		return nil, err
	}
	var result []Position
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("decoding positions: %w", err)
	}
	return result, nil
}

// GetClosed fetches closed positions.
func (c *Client) GetClosed() ([]ClosedPosition, error) {
	data, err := c.doRequest("/closed")
	if err != nil {
		return nil, err
	}
	var result []ClosedPosition
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("decoding closed positions: %w", err)
	}
	return result, nil
}

// GetLogs fetches recent log entries.
func (c *Client) GetLogs() ([]LogEntry, error) {
	data, err := c.doRequest("/logs")
	if err != nil {
		return nil, err
	}
	var result []LogEntry
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("decoding logs: %w", err)
	}
	return result, nil
}

// GetMode fetches the operating mode.
func (c *Client) GetMode() (*ModeResponse, error) {
	data, err := c.doRequest("/mode")
	if err != nil {
		return nil, err
	}
	var result ModeResponse
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("decoding mode: %w", err)
	}
	return &result, nil
}
