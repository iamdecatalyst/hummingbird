// Package cricket provides a client for the Cricket Protocol API.
// Cricket powers all token risk analysis and smart-money signal detection
// in Hummingbird — users need a Cricket account to run the bot.
// Sign up at https://cricket.vylth.com
package cricket

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// ErrNoCricketKey is returned when a Cricket API call is attempted without a key.
var ErrNoCricketKey = errors.New("CRICKET_API_KEY not set — sign up at https://cricket.vylth.com")

// ErrTokenNotFound is returned by Mantis when the mint address doesn't exist on-chain
// (failed launch, unconfirmed tx, or wrong account type). Callers should skip the token.
var ErrTokenNotFound = errors.New("token not found or not a valid mint")

// Client is an HTTP client for the Cricket Protocol API.
type Client struct {
	baseURL string
	apiKey  string
	http    *http.Client
}

// New creates a Cricket API client.
// baseURL is the Cricket API root (e.g. https://api-cricket.vylth.com).
// apiKey is the Cricket API key from your dashboard.
func New(baseURL, apiKey string) *Client {
	return &Client{
		baseURL: baseURL,
		apiKey:  apiKey,
		http:    &http.Client{Timeout: 8 * time.Second},
	}
}

// MantisScan returns a full risk scan for a token address.
// Checks mint/freeze authority, holder concentration, deployer age, and more.
// devWallet and bondingCurve are optional — pass empty string to omit.
// Maps to GET /api/cricket/mantis/scan/{token}?dev_wallet=&bonding_curve=
func (c *Client) MantisScan(ctx context.Context, tokenAddress, devWallet, bondingCurve string) (*MantisScanResponse, error) {
	url := fmt.Sprintf("%s/api/cricket/mantis/scan/%s", c.baseURL, tokenAddress)
	if devWallet != "" || bondingCurve != "" {
		params := "?"
		if devWallet != "" {
			params += "dev_wallet=" + devWallet
		}
		if bondingCurve != "" {
			if devWallet != "" {
				params += "&"
			}
			params += "bonding_curve=" + bondingCurve
		}
		url += params
	}
	return doGet[MantisScanResponse](ctx, c, url)
}

// FireflyWallet returns a wallet profile and smart-money score.
// Used to evaluate the dev wallet before entering a position.
// Maps to GET /api/cricket/firefly/wallet/{address}
func (c *Client) FireflyWallet(ctx context.Context, address string) (*FireflyWalletResponse, error) {
	url := fmt.Sprintf("%s/api/cricket/firefly/wallet/%s", c.baseURL, address)
	return doGet[FireflyWalletResponse](ctx, c, url)
}

// FireflySignals returns current accumulation/exodus signals detected by Cricket.
// Used by the scalper to find second-wave entries and rug warnings in open positions.
// Maps to GET /api/cricket/firefly/signals
func (c *Client) FireflySignals(ctx context.Context) (*FireflySignalsResponse, error) {
	url := fmt.Sprintf("%s/api/cricket/firefly/signals", c.baseURL)
	return doGet[FireflySignalsResponse](ctx, c, url)
}

func doGet[T any](ctx context.Context, c *Client, url string) (*T, error) {
	if c.apiKey == "" {
		return nil, ErrNoCricketKey
	}
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("User-Agent", "hummingbird/1.0")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == 422 {
		// 404 TOKEN_NOT_FOUND or 422 NOT_A_TOKEN_MINT — address doesn't exist or isn't a mint.
		// Clean skip — not an error worth logging loudly.
		return nil, ErrTokenNotFound
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("cricket API %d: %s", resp.StatusCode, string(body))
	}

	var result T
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("decode: %w (body: %.200s)", err, string(body))
	}
	return &result, nil
}
