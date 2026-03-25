use std::time::{SystemTime, UNIX_EPOCH};

use anyhow::Result;
use futures_util::{SinkExt, StreamExt};
use serde_json::{json, Value};
use tokio::sync::mpsc;
use tokio_tungstenite::{connect_async, tungstenite::Message};
use tracing::{debug, error, info, warn};

use crate::config::Config;
use crate::fetcher::TransactionFetcher;
use crate::forwarder::Forwarder;
use crate::parser::is_new_token_launch;
use crate::types::{LogsNotification, TokenDetected};

/// Runs the listener forever, reconnecting on disconnect or error
pub async fn run(config: Config) -> Result<()> {
    let fetcher = TransactionFetcher::new(config.rpc_http.clone());
    let forwarder = Forwarder::new(config.scorer_url.clone());

    loop {
        match run_once(&config, &fetcher, &forwarder).await {
            Ok(_) => warn!("WebSocket closed — reconnecting in 2s..."),
            Err(e) => error!("Listener error: {} — reconnecting in 2s...", e),
        }
        tokio::time::sleep(std::time::Duration::from_secs(2)).await;
    }
}

async fn run_once(
    config: &Config,
    fetcher: &TransactionFetcher,
    forwarder: &Forwarder,
) -> Result<()> {
    info!("Connecting to {}", config.rpc_ws);

    let (ws_stream, _) = connect_async(&config.rpc_ws).await?;
    let (mut write, mut read) = ws_stream.split();

    // Subscribe to all logs mentioning the pump.fun program
    let subscribe = json!({
        "jsonrpc": "2.0",
        "id": 1,
        "method": "logsSubscribe",
        "params": [
            { "mentions": [config.pump_fun_program] },
            { "commitment": "processed" }
        ]
    });
    write.send(Message::Text(subscribe.to_string())).await?;
    info!("Subscribed — watching pump.fun for new launches");

    // Unbounded channel: WebSocket reader pushes (signature, slot), worker fetches + forwards
    // This keeps the WebSocket reader non-blocking — we never stall waiting for RPC calls
    let (tx, mut rx) = mpsc::unbounded_channel::<(String, u64)>();

    // Spawn the fetch + forward worker
    let fetcher_url = config.rpc_http.clone();
    let scorer_url = config.scorer_url.clone();
    tokio::spawn(async move {
        let fetcher = TransactionFetcher::new(fetcher_url);
        let forwarder = Forwarder::new(scorer_url);

        while let Some((signature, slot)) = rx.recv().await {
            let timestamp_ms = SystemTime::now()
                .duration_since(UNIX_EPOCH)
                .unwrap()
                .as_millis() as u64;

            match fetcher.fetch_accounts(&signature).await {
                Ok(Some(accounts)) => {
                    let token = TokenDetected {
                        mint: accounts.mint.clone(),
                        signature: signature.clone(),
                        dev_wallet: accounts.dev_wallet,
                        bonding_curve: accounts.bonding_curve,
                        timestamp_ms,
                        slot,
                    };
                    info!("🐦  NEW TOKEN  mint={}  dev={}", token.mint, token.dev_wallet);
                    let _ = forwarder.forward(&token).await;
                }
                Ok(None) => debug!("Skipped {} — unexpected account layout", signature),
                Err(e) => error!("Fetch error for {}: {}", signature, e),
            }
        }
    });

    // Main WebSocket read loop
    while let Some(msg) = read.next().await {
        match msg? {
            Message::Text(text) => {
                let value: Value = match serde_json::from_str(&text) {
                    Ok(v) => v,
                    Err(_) => continue,
                };

                // Subscription confirmation — skip
                if value.get("result").is_some() && value.get("method").is_none() {
                    continue;
                }

                if let Ok(notification) = serde_json::from_value::<LogsNotification>(value) {
                    if let Some(params) = notification.params {
                        let logs_value = &params.result.value;
                        let slot = params.result.context.slot;

                        if is_new_token_launch(logs_value) {
                            let _ = tx.send((logs_value.signature.clone(), slot));
                        }
                    }
                }
            }
            Message::Ping(data) => {
                write.send(Message::Pong(data)).await?;
            }
            Message::Close(_) => {
                warn!("Server closed the WebSocket connection");
                break;
            }
            _ => {}
        }
    }

    Ok(())
}
