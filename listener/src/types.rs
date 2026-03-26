use serde::{Deserialize, Serialize};

/// A new token detected on any supported launchpad — forwarded to the Python scorer
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct TokenDetected {
    pub mint: String,           // token mint / contract address
    pub signature: String,      // tx signature / hash
    pub dev_wallet: String,     // creator wallet
    pub bonding_curve: String,  // bonding curve account (empty string for EVM)
    pub timestamp_ms: u64,
    pub slot: u64,              // Solana slot (0 for EVM)
    pub platform: String,       // "pump_fun" | "moonshot" | "four_meme" | "virtuals" etc.
    pub chain: String,          // "solana" | "base" | "bnb"
}

// --- Solana WebSocket notification types ---

#[derive(Debug, Deserialize)]
pub struct LogsNotification {
    pub method: Option<String>,
    pub params: Option<LogsParams>,
}

#[derive(Debug, Deserialize)]
pub struct LogsParams {
    pub result: LogsResult,
    pub subscription: u64,
}

#[derive(Debug, Deserialize)]
pub struct LogsResult {
    pub context: Context,
    pub value: LogsValue,
}

#[derive(Debug, Deserialize)]
pub struct Context {
    pub slot: u64,
}

#[derive(Debug, Deserialize)]
pub struct LogsValue {
    pub signature: String,
    pub err: Option<serde_json::Value>,
    pub logs: Vec<String>,
}
