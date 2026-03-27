package cricket

// TokenDetected is the payload sent by the Rust listener when a new token is detected.
// Matches the Rust forwarder struct exactly.
type TokenDetected struct {
	Mint         string `json:"mint"`
	DevWallet    string `json:"dev_wallet"`
	BondingCurve string `json:"bonding_curve"`
	Platform     string `json:"platform"` // pump_fun | moonshot | raydium_launchlab | boop | etc.
	Chain        string `json:"chain"`    // solana | base | bnb
	TimestampMs  int64  `json:"timestamp_ms"`
}

// MantisScanResponse is the response from GET /api/cricket/mantis/scan/{token}.
type MantisScanResponse struct {
	Success bool `json:"success"`
	Data    struct {
		Scan struct {
			TokenAddress           string  `json:"token_address"`
			MintAuthorityRevoked   bool    `json:"mint_authority_revoked"`
			FreezeAuthorityRevoked bool    `json:"freeze_authority_revoked"`
			LPLocked               bool    `json:"lp_locked"`
			LPLockDurationDays     *int    `json:"lp_lock_duration_days"`
			Top10HolderPct         float64 `json:"top_10_holder_pct"`
			DeployerWalletAgeDays  int     `json:"deployer_wallet_age_days"`
			MetadataMutable        bool    `json:"metadata_mutable"`
			Flags                  []Flag  `json:"flags"`
		} `json:"scan"`
		RiskScore RiskScore `json:"risk_score"`
	} `json:"data"`
}

// RiskScore is the scored output from Mantis.
type RiskScore struct {
	Score     int    `json:"score"`
	Rating    string `json:"rating"` // low | moderate | high | critical
	ScannedAt string `json:"scanned_at"`
}

// Flag is a risk finding from Mantis.
type Flag struct {
	Check    string `json:"check"`
	Severity string `json:"severity"` // low | medium | high | critical
	Status   string `json:"status"`
	Detail   string `json:"detail"`
}

// FireflyWalletResponse is the response from GET /api/cricket/firefly/wallet/{address}.
type FireflyWalletResponse struct {
	Success bool `json:"success"`
	Data    struct {
		Address      string  `json:"address"`
		Chain        string  `json:"chain"`
		Score        int     `json:"score"`
		TotalTrades  int     `json:"total_trades"`
		WinRate      float64 `json:"win_rate"`
		AvgReturnPct float64 `json:"avg_return_pct"`
		TotalPnLUSD  float64 `json:"total_pnl_usd"`
		Style        string  `json:"style"` // early_accumulator | momentum_trader | sniper | smart_contract_deployer | unknown
		ActiveSince  string  `json:"active_since"`
	} `json:"data"`
}

// FireflySignal is a market signal detected by Cricket Firefly.
type FireflySignal struct {
	SignalType   string `json:"signal_type"` // accumulation | divergence | exodus
	TokenAddress string `json:"token_address"`
	TokenSymbol  string `json:"token_symbol"`
	Strength     string `json:"strength"` // weak | moderate | strong
	Evidence     struct {
		SmartWalletsCount int     `json:"smart_wallets_count"`
		AvgWalletScore    int     `json:"avg_wallet_score"`
		TotalVolumeUSD    float64 `json:"total_volume_usd"`
		TimeWindowHours   int     `json:"time_window_hours"`
	} `json:"evidence"`
	DetectedAt string `json:"detected_at"`
}

// FireflySignalsResponse is the response from GET /api/cricket/firefly/signals.
type FireflySignalsResponse struct {
	Success bool            `json:"success"`
	Data    []FireflySignal `json:"data"`
}
