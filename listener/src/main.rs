mod config;
mod fetcher;
mod forwarder;
mod listener;
mod parser;
mod types;

use anyhow::Result;
use tracing::info;

#[tokio::main]
async fn main() -> Result<()> {
    tracing_subscriber::fmt()
        .with_env_filter(
            tracing_subscriber::EnvFilter::from_default_env()
                .add_directive("hummingbird_listener=info".parse()?),
        )
        .init();

    let config = config::Config::from_env()?;

    info!("🐦 Hummingbird Listener");
    info!("   Solana RPC : {}", config.solana_ws);
    info!("   Scorer     : {}", config.scorer_url);
    info!(
        "   Platforms  : {}",
        config
            .solana_programs
            .iter()
            .map(|p| p.platform.as_str())
            .chain(config.evm_chains.iter().map(|c| c.platform.as_str()))
            .collect::<Vec<_>>()
            .join(", ")
    );

    listener::run(config).await
}
