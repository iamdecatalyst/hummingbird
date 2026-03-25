use anyhow::Result;

pub struct Config {
    /// Solana WebSocket RPC endpoint
    pub rpc_ws: String,
    /// Solana HTTP RPC endpoint (for fetching full transactions)
    pub rpc_http: String,
    /// Python scorer service URL
    pub scorer_url: String,
    /// pump.fun program address on Solana mainnet
    pub pump_fun_program: String,
}

impl Config {
    pub fn from_env() -> Result<Self> {
        Ok(Config {
            rpc_ws: std::env::var("RPC_WS")
                .unwrap_or_else(|_| "wss://api.mainnet-beta.solana.com".to_string()),
            rpc_http: std::env::var("RPC_HTTP")
                .unwrap_or_else(|_| "https://api.mainnet-beta.solana.com".to_string()),
            scorer_url: std::env::var("SCORER_URL")
                .unwrap_or_else(|_| "http://localhost:8001".to_string()),
            pump_fun_program: std::env::var("PUMP_FUN_PROGRAM")
                .unwrap_or_else(|_| "6EF8rrecthR5Dkzon8Nwu78hRvfCKubJ14M5uBEwF6P".to_string()),
        })
    }
}
