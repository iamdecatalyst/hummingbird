use anyhow::Result;
use tracing::{debug, error};

use crate::types::TokenDetected;

/// Forwards detected tokens to the Python scorer via HTTP POST
pub struct Forwarder {
    scorer_url: String,
    scorer_secret: String,
    client: reqwest::Client,
}

impl Forwarder {
    pub fn scorer_url(&self) -> String {
        self.scorer_url.clone()
    }

    pub fn scorer_secret(&self) -> String {
        self.scorer_secret.clone()
    }

    pub fn new(scorer_url: String, scorer_secret: String) -> Self {
        Self {
            scorer_url,
            scorer_secret,
            client: reqwest::Client::builder()
                .timeout(std::time::Duration::from_secs(3))
                .build()
                .unwrap(),
        }
    }

    pub async fn forward(&self, token: &TokenDetected) -> Result<()> {
        let url = format!("{}/score", self.scorer_url);

        let req = self
            .client
            .post(&url)
            .bearer_auth(&self.scorer_secret)
            .json(token);

        match req.send().await {
            Ok(resp) if resp.status().is_success() => {
                debug!("Forwarded {} to scorer", token.mint);
            }
            Ok(resp) => {
                // Scorer returned an error — log and continue, don't crash the pipeline
                error!(
                    "Scorer returned {} for mint {}",
                    resp.status(),
                    token.mint
                );
            }
            Err(e) => {
                // Scorer is down or unreachable — log and continue
                error!("Failed to reach scorer for {}: {}", token.mint, e);
            }
        }

        Ok(())
    }
}
