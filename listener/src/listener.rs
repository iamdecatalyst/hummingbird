use std::sync::Arc;
use std::time::{SystemTime, UNIX_EPOCH};

use anyhow::Result;
use futures_util::{SinkExt, StreamExt};
use serde_json::{json, Value};
use tokio::sync::{mpsc, Semaphore};
use tokio_tungstenite::{connect_async, tungstenite::Message};
use tracing::{debug, error, info, warn};

use crate::config::{Config, EvmChain, SolanaProgram};
use crate::fetcher::TransactionFetcher;
use crate::forwarder::Forwarder;
use crate::parser::is_new_token_launch;
use crate::types::{LogsNotification, TokenDetected};

/// Spawns all chain listeners concurrently and runs forever.
pub async fn run(config: Config) -> Result<()> {
    let forwarder = Forwarder::new(config.scorer_url.clone(), config.scorer_secret.clone());

    // Shared semaphore — limits total concurrent getTransaction RPC calls across ALL platforms.
    // Helius free tier is ~10 RPS; 3 concurrent calls at ~300ms each ≈ 10 RPS max.
    let rpc_sem = Arc::new(Semaphore::new(3));

    let mut handles = Vec::new();

    // One Solana listener per program (each gets its own subscription + reconnect loop)
    for program in config.solana_programs.clone() {
        let ws = config.solana_ws.clone();
        let http = config.solana_http.clone();
        let fwd = Forwarder::new(config.scorer_url.clone(), config.scorer_secret.clone());
        let sem = rpc_sem.clone();
        let h = tokio::spawn(async move {
            run_solana_program(ws, http, program, fwd, sem).await;
        });
        handles.push(h);
    }

    // One EVM listener per chain/platform
    for chain in config.evm_chains.clone() {
        let fwd = Forwarder::new(config.scorer_url.clone(), config.scorer_secret.clone());
        let h = tokio::spawn(async move {
            run_evm_chain(chain, fwd).await;
        });
        handles.push(h);
    }

    if handles.is_empty() {
        anyhow::bail!("No platforms enabled — set ENABLE_PUMP_FUN=true or other flags");
    }

    // Wait for all (they run forever, so this blocks indefinitely)
    for h in handles {
        let _ = h.await;
    }

    let _ = forwarder; // keep alive
    Ok(())
}

// ─── Solana listener ─────────────────────────────────────────────────────────

async fn run_solana_program(
    ws_url: String,
    http_url: String,
    program: SolanaProgram,
    forwarder: Forwarder,
    rpc_sem: Arc<Semaphore>,
) {
    loop {
        info!("[{}] connecting to Solana WebSocket", program.platform);
        match solana_once(&ws_url, &http_url, &program, &forwarder, rpc_sem.clone()).await {
            Ok(_) => warn!("[{}] WebSocket closed — reconnecting in 2s", program.platform),
            Err(e) => error!("[{}] error: {} — reconnecting in 2s", program.platform, e),
        }
        tokio::time::sleep(std::time::Duration::from_secs(2)).await;
    }
}

async fn solana_once(
    ws_url: &str,
    http_url: &str,
    program: &SolanaProgram,
    forwarder: &Forwarder,
    rpc_sem: Arc<Semaphore>,
) -> Result<()> {
    let (ws_stream, _) = connect_async(ws_url).await?;
    let (mut write, mut read) = ws_stream.split();

    let subscribe = json!({
        "jsonrpc": "2.0",
        "id": 1,
        "method": "logsSubscribe",
        "params": [
            { "mentions": [program.program_id] },
            { "commitment": "processed" }
        ]
    });
    write.send(Message::Text(subscribe.to_string())).await?;
    info!("[{}] subscribed — program {}", program.platform, &program.program_id[..8]);

    // Bounded channel — if the fetcher can't keep up, drop oldest events rather than
    // queue forever. Stale tokens (>queue depth old) aren't worth entering anyway.
    let (tx, mut rx) = mpsc::channel::<(String, u64)>(8);

    // Fetch + forward worker — acquires shared semaphore before each RPC call.
    let http = http_url.to_string();
    let fwd = Forwarder::new(forwarder.scorer_url(), forwarder.scorer_secret());
    let platform = program.platform.clone();
    tokio::spawn(async move {
        let fetcher = TransactionFetcher::new(http);
        while let Some((signature, slot)) = rx.recv().await {
            // Acquire shared RPC slot — blocks if 3 calls already in flight across all platforms.
            let _permit = rpc_sem.acquire().await.unwrap();
            let timestamp_ms = now_ms();
            match fetcher.fetch_accounts(&signature, &platform).await {
                Ok(Some(accounts)) => {
                    let token = TokenDetected {
                        mint: accounts.mint.clone(),
                        signature: signature.clone(),
                        dev_wallet: accounts.dev_wallet,
                        bonding_curve: accounts.bonding_curve,
                        timestamp_ms,
                        slot,
                        platform: platform.clone(),
                        chain: "solana".to_string(),
                    };
                    info!(
                        "🐦  [{}] mint={}  dev={}",
                        token.platform,
                        &token.mint[..8],
                        &token.dev_wallet[..8]
                    );
                    let _ = fwd.forward(&token).await;
                }
                Ok(None) => debug!("[{}] skipped {} — parse failed", platform, &signature[..8]),
                Err(e) => error!("[{}] fetch error {}: {}", platform, &signature[..8], e),
            }
            // permit dropped here, freeing the RPC slot
        }
    });

    while let Some(msg) = read.next().await {
        match msg? {
            Message::Text(text) => {
                let value: Value = match serde_json::from_str(&text) {
                    Ok(v) => v,
                    Err(_) => continue,
                };
                if value.get("result").is_some() && value.get("method").is_none() {
                    continue; // subscription confirmation
                }
                if let Ok(notification) = serde_json::from_value::<LogsNotification>(value) {
                    if let Some(params) = notification.params {
                        let logs_value = &params.result.value;
                        let slot = params.result.context.slot;
                        if is_new_token_launch(logs_value) {
                            if tx.try_send((logs_value.signature.clone(), slot)).is_err() {
                                debug!("[{}] fetch queue full — dropping {}", program.platform, &logs_value.signature[..8]);
                            }
                        }
                    }
                }
            }
            Message::Ping(data) => {
                write.send(Message::Pong(data)).await?;
            }
            Message::Close(_) => break,
            _ => {}
        }
    }
    Ok(())
}

// ─── EVM listener ─────────────────────────────────────────────────────────────

async fn run_evm_chain(chain: EvmChain, forwarder: Forwarder) {
    loop {
        info!("[{}:{}] connecting to EVM WebSocket", chain.chain, chain.platform);
        match evm_once(&chain, &forwarder).await {
            Ok(_) => warn!("[{}:{}] WebSocket closed — reconnecting in 2s", chain.chain, chain.platform),
            Err(e) => error!("[{}:{}] error: {} — reconnecting in 2s", chain.chain, chain.platform, e),
        }
        tokio::time::sleep(std::time::Duration::from_secs(2)).await;
    }
}

async fn evm_once(chain: &EvmChain, forwarder: &Forwarder) -> Result<()> {
    let (ws_stream, _) = connect_async(&chain.ws_url).await?;
    let (mut write, mut read) = ws_stream.split();

    // Subscribe to logs from the factory contract matching the TokenCreated topic
    let subscribe = json!({
        "jsonrpc": "2.0",
        "id": 1,
        "method": "eth_subscribe",
        "params": [
            "logs",
            {
                "address": chain.factory_address,
                "topics": [chain.create_topic]
            }
        ]
    });
    write.send(Message::Text(subscribe.to_string())).await?;
    info!(
        "[{}:{}] subscribed — factory {}",
        chain.chain,
        chain.platform,
        &chain.factory_address[..10]
    );

    while let Some(msg) = read.next().await {
        match msg? {
            Message::Text(text) => {
                let value: Value = match serde_json::from_str(&text) {
                    Ok(v) => v,
                    Err(_) => continue,
                };

                // Subscription confirmation
                if value.get("result").is_some() && value.get("method").is_none() {
                    continue;
                }

                // Log notification
                if let Some(log) = value
                    .get("params")
                    .and_then(|p| p.get("result"))
                {
                    if let Some(token) = parse_evm_log(log, chain) {
                        info!(
                            "🐦  [{}:{}] contract={}",
                            chain.chain,
                            chain.platform,
                            &token.mint[..10]
                        );
                        let _ = forwarder.forward(&token).await;
                    }
                }
            }
            Message::Ping(data) => {
                write.send(Message::Pong(data)).await?;
            }
            Message::Close(_) => break,
            _ => {}
        }
    }
    Ok(())
}

/// Parses an EVM log event into a TokenDetected.
/// Each launchpad emits different event data — this extracts what we can from the log.
fn parse_evm_log(log: &Value, chain: &EvmChain) -> Option<TokenDetected> {
    // The token contract address is typically in `topics[1]` or the `address` field
    // depending on the launchpad. `data` contains additional fields.
    // This is a best-effort parse — scorer handles missing fields gracefully.

    let tx_hash = log.get("transactionHash")?.as_str()?.to_string();
    let block_number = log
        .get("blockNumber")
        .and_then(|b| b.as_str())
        .and_then(|b| u64::from_str_radix(b.trim_start_matches("0x"), 16).ok())
        .unwrap_or(0);

    // Token address: for most EVM launchpads, topics[1] is the new token address
    let topics = log.get("topics")?.as_array()?;
    let token_address = if topics.len() > 1 {
        // topics are 32-byte padded — last 20 bytes = address
        let raw = topics[1].as_str().unwrap_or("");
        format!("0x{}", &raw[raw.len().saturating_sub(40)..])
    } else {
        return None;
    };

    // Creator: topics[2] if present, else tx sender (we don't have that here)
    let creator = if topics.len() > 2 {
        let raw = topics[2].as_str().unwrap_or("");
        format!("0x{}", &raw[raw.len().saturating_sub(40)..])
    } else {
        "unknown".to_string()
    };

    Some(TokenDetected {
        mint: token_address,
        signature: tx_hash,
        dev_wallet: creator,
        bonding_curve: String::new(), // EVM launchpads use different curve structures
        timestamp_ms: now_ms(),
        slot: block_number,
        platform: chain.platform.clone(),
        chain: chain.chain.clone(),
    })
}

fn now_ms() -> u64 {
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .unwrap()
        .as_millis() as u64
}
