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
    info!("   RPC WebSocket : {}", config.rpc_ws);
    info!("   RPC HTTP      : {}", config.rpc_http);
    info!("   Scorer        : {}", config.scorer_url);
    info!("   Program       : {}", config.pump_fun_program);

    listener::run(config).await
}
