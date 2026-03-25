use anyhow::Result;
use serde_json::{json, Value};
use tracing::{debug, warn};

/// Fetches the full transaction from the RPC to extract token accounts
pub struct TransactionFetcher {
    rpc_url: String,
    client: reqwest::Client,
}

pub struct TransactionAccounts {
    pub mint: String,
    pub bonding_curve: String,
    pub dev_wallet: String,
}

impl TransactionFetcher {
    pub fn new(rpc_url: String) -> Self {
        Self {
            rpc_url,
            client: reqwest::Client::builder()
                .timeout(std::time::Duration::from_secs(5))
                .build()
                .unwrap(),
        }
    }

    /// Fetches the transaction and extracts the key accounts.
    ///
    /// pump.fun Create instruction account layout:
    ///   [0] mint
    ///   [1] mint authority
    ///   [2] bonding curve
    ///   [3] associated bonding curve
    ///   [4] global
    ///   [5] mpl token metadata program
    ///   [6] metadata
    ///   [7] user (dev / creator)
    ///   [8..] system accounts
    pub async fn fetch_accounts(&self, signature: &str) -> Result<Option<TransactionAccounts>> {
        let body = json!({
            "jsonrpc": "2.0",
            "id": 1,
            "method": "getTransaction",
            "params": [
                signature,
                {
                    "encoding": "json",
                    "commitment": "confirmed",
                    "maxSupportedTransactionVersion": 0
                }
            ]
        });

        let resp: Value = self
            .client
            .post(&self.rpc_url)
            .json(&body)
            .send()
            .await?
            .json()
            .await?;

        let accounts: Option<Vec<String>> = resp["result"]["transaction"]["message"]["accountKeys"]
            .as_array()
            .map(|arr| {
                arr.iter()
                    .map(|v| v.as_str().unwrap_or("").to_string())
                    .collect()
            });

        match accounts {
            Some(accs) if accs.len() >= 8 => Ok(Some(TransactionAccounts {
                mint: accs[0].clone(),
                bonding_curve: accs[2].clone(),
                dev_wallet: accs[7].clone(),
            })),
            Some(accs) => {
                debug!(
                    "Unexpected account layout for {} ({} accounts)",
                    signature,
                    accs.len()
                );
                Ok(None)
            }
            None => {
                warn!("Could not parse accounts for {}", signature);
                Ok(None)
            }
        }
    }
}
