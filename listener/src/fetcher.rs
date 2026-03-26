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
    /// Account layouts per platform:
    ///
    /// pump_fun (legacy tx) — message.accountKeys order (fee payer first):
    ///   [0] user / dev (fee payer, signer)
    ///   [1] mint (new token keypair, signer)
    ///   [2] mintAuthority PDA (writable non-signer)
    ///   [3] bondingCurve PDA (writable non-signer)
    ///   [4] associatedBondingCurve PDA (writable non-signer)
    ///   [5] global PDA (writable non-signer)
    ///   [6] metadata PDA (writable non-signer)
    ///   [7..] readonly non-signers (programs, system, etc.)
    ///   min: 4 accounts (need indices 0-3)
    ///
    /// raydium_launchlab (V0 tx):
    ///   [0] dev wallet (fee payer / creator)
    ///   [2] pool state (bonding curve proxy)
    ///   [4] mint
    ///   min: 5 accounts
    ///
    /// boop (V0 tx):
    ///   [0] dev wallet
    ///   [1] mint
    ///   [2] bonding curve
    ///   min: 3 accounts
    pub async fn fetch_accounts(
        &self,
        signature: &str,
        platform: &str,
    ) -> Result<Option<TransactionAccounts>> {
        let body = json!({
            "jsonrpc": "2.0",
            "id": 1,
            "method": "getTransaction",
            "params": [
                signature,
                {
                    "encoding": "jsonParsed",
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

        // jsonParsed returns account objects { pubkey, signer, writable, source }
        // for both legacy and V0 (versioned) transactions.
        let accounts: Option<Vec<String>> = resp["result"]["transaction"]["message"]["accountKeys"]
            .as_array()
            .map(|arr| {
                arr.iter()
                    .map(|v| {
                        v.get("pubkey")
                            .and_then(|p| p.as_str())
                            .unwrap_or_else(|| v.as_str().unwrap_or(""))
                            .to_string()
                    })
                    .collect()
            });

        // Check for RPC-level errors (rate limit, etc.)
        if let Some(err) = resp.get("error") {
            warn!("RPC error for {} ({}): {}", &signature[..12], platform, err);
            return Ok(None);
        }

        if resp["result"].is_null() {
            debug!("Transaction not found (not yet confirmed?) for {} ({})", &signature[..12], platform);
            return Ok(None);
        }

        let accs = match accounts {
            Some(a) if !a.is_empty() => a,
            _ => {
                warn!("Could not parse accounts for {} ({})", &signature[..12], platform);
                return Ok(None);
            }
        };

        let result = match platform {
            "pump_fun" => {
                if accs.len() < 4 {
                    debug!("[pump_fun] unexpected account count {} for {}", accs.len(), signature);
                    return Ok(None);
                }
                TransactionAccounts {
                    dev_wallet: accs[0].clone(),  // fee payer (first signer)
                    mint: accs[1].clone(),         // new token keypair (second signer)
                    bonding_curve: accs[3].clone(), // bondingCurve PDA (after mintAuthority)
                }
            }
            "raydium_launchlab" => {
                if accs.len() < 5 {
                    debug!("[raydium_launchlab] unexpected account count {} for {}", accs.len(), signature);
                    return Ok(None);
                }
                TransactionAccounts {
                    dev_wallet: accs[0].clone(),
                    bonding_curve: accs[2].clone(), // pool state — used as proxy
                    mint: accs[4].clone(),
                }
            }
            "boop" => {
                if accs.len() < 3 {
                    debug!("[boop] unexpected account count {} for {}", accs.len(), signature);
                    return Ok(None);
                }
                TransactionAccounts {
                    dev_wallet: accs[0].clone(),
                    mint: accs[1].clone(),
                    bonding_curve: accs[2].clone(),
                }
            }
            _ => {
                // Unknown platform — use pump_fun layout as fallback
                if accs.len() < 8 {
                    debug!("[{}] unexpected account count {} for {}", platform, accs.len(), signature);
                    return Ok(None);
                }
                TransactionAccounts {
                    mint: accs[0].clone(),
                    bonding_curve: accs[2].clone(),
                    dev_wallet: accs[7].clone(),
                }
            }
        };

        Ok(Some(result))
    }
}
