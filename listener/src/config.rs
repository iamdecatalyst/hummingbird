use anyhow::Result;

/// A Solana launchpad program to monitor.
#[derive(Debug, Clone)]
pub struct SolanaProgram {
    pub program_id: String,
    pub platform: String, // "pump_fun" | "moonshot" | "raydium_launchlab" | "boop"
}

/// An EVM chain + launchpad factory to monitor.
#[derive(Debug, Clone)]
pub struct EvmChain {
    pub ws_url: String,
    pub http_url: String,
    pub chain: String,          // "base" | "bnb"
    pub factory_address: String,
    pub platform: String,       // "pump_fun_base" | "four_meme" | "virtuals"
    pub create_topic: String,   // the event topic hash for TokenCreated
}

pub struct Config {
    // Solana
    pub solana_ws: String,
    pub solana_http: String,
    pub solana_programs: Vec<SolanaProgram>,

    // EVM chains (Base, BNB)
    pub evm_chains: Vec<EvmChain>,

    // Scorer
    pub scorer_url: String,
}

impl Config {
    pub fn from_env() -> Result<Self> {
        Ok(Config {
            solana_ws: env("RPC_WS", "wss://api.mainnet-beta.solana.com"),
            solana_http: env("RPC_HTTP", "https://api.mainnet-beta.solana.com"),

            solana_programs: solana_programs_from_env(),

            evm_chains: evm_chains_from_env(),

            scorer_url: env("SCORER_URL", "http://localhost:8001"),
        })
    }
}

fn solana_programs_from_env() -> Vec<SolanaProgram> {
    let mut programs = Vec::new();

    // pump.fun — always enabled by default
    if env_flag("ENABLE_PUMP_FUN", true) {
        programs.push(SolanaProgram {
            program_id: env(
                "PUMP_FUN_PROGRAM",
                "6EF8rrecthR5Dkzon8Nwu78hRvfCKubJ14M5uBEwF6P",
            ),
            platform: "pump_fun".to_string(),
        });
    }

    // Moonshot (DEX Screener's launchpad)
    if env_flag("ENABLE_MOONSHOT", false) {
        programs.push(SolanaProgram {
            program_id: env(
                "MOONSHOT_PROGRAM",
                "MoonCVVNZFSYkqNXP6bxHLPL6QQJiMagDL3qcqUQTrG",
            ),
            platform: "moonshot".to_string(),
        });
    }

    // Raydium LaunchLab
    if env_flag("ENABLE_RAYDIUM_LAUNCHLAB", false) {
        programs.push(SolanaProgram {
            program_id: env(
                "RAYDIUM_LAUNCHLAB_PROGRAM",
                "LanMV9sAd7wArD4vsbMervnLNMmHVCBYYMBVAGHPRbm",
            ),
            platform: "raydium_launchlab".to_string(),
        });
    }

    // Boop.fun
    if env_flag("ENABLE_BOOP", false) {
        programs.push(SolanaProgram {
            program_id: env(
                "BOOP_PROGRAM",
                "boopkpWqe68MSxLqBGogs8ZbUDN4GXaLhFwNcMWrS8D",
            ),
            platform: "boop".to_string(),
        });
    }

    programs
}

fn evm_chains_from_env() -> Vec<EvmChain> {
    let mut chains = Vec::new();

    // pump.fun on Base
    if env_flag("ENABLE_BASE_PUMP_FUN", false) {
        let ws = std::env::var("BASE_RPC_WS").unwrap_or_default();
        let http = std::env::var("BASE_RPC_HTTP").unwrap_or_default();
        if !ws.is_empty() {
            chains.push(EvmChain {
                ws_url: ws,
                http_url: http,
                chain: "base".to_string(),
                // pump.fun Base factory contract
                factory_address: env(
                    "BASE_PUMP_FUN_FACTORY",
                    "0x0000000000000000000000000000000000000000", // placeholder — set in .env
                ),
                platform: "pump_fun_base".to_string(),
                // TokenCreated(address token, address creator, ...)
                create_topic: env(
                    "BASE_PUMP_FUN_TOPIC",
                    "0x0000000000000000000000000000000000000000000000000000000000000000",
                ),
            });
        }
    }

    // Four.meme on BNB
    if env_flag("ENABLE_FOUR_MEME", false) {
        let ws = std::env::var("BNB_RPC_WS").unwrap_or_default();
        let http = std::env::var("BNB_RPC_HTTP").unwrap_or_default();
        if !ws.is_empty() {
            chains.push(EvmChain {
                ws_url: ws,
                http_url: http,
                chain: "bnb".to_string(),
                factory_address: env(
                    "FOUR_MEME_FACTORY",
                    "0x0000000000000000000000000000000000000000",
                ),
                platform: "four_meme".to_string(),
                create_topic: env(
                    "FOUR_MEME_TOPIC",
                    "0x0000000000000000000000000000000000000000000000000000000000000000",
                ),
            });
        }
    }

    // Virtuals Protocol on Base (AI agent tokens)
    if env_flag("ENABLE_VIRTUALS", false) {
        let ws = std::env::var("BASE_RPC_WS").unwrap_or_default();
        let http = std::env::var("BASE_RPC_HTTP").unwrap_or_default();
        if !ws.is_empty() {
            chains.push(EvmChain {
                ws_url: ws,
                http_url: http,
                chain: "base".to_string(),
                factory_address: env(
                    "VIRTUALS_FACTORY",
                    "0x0000000000000000000000000000000000000000",
                ),
                platform: "virtuals".to_string(),
                create_topic: env(
                    "VIRTUALS_TOPIC",
                    "0x0000000000000000000000000000000000000000000000000000000000000000",
                ),
            });
        }
    }

    chains
}

fn env(key: &str, default: &str) -> String {
    std::env::var(key).unwrap_or_else(|_| default.to_string())
}

fn env_flag(key: &str, default: bool) -> bool {
    match std::env::var(key).as_deref() {
        Ok("true") | Ok("1") | Ok("yes") => true,
        Ok("false") | Ok("0") | Ok("no") => false,
        _ => default,
    }
}
