use crate::types::LogsValue;

/// pump.fun emits this log line when a new token is created
const PUMPFUN_CREATE: &str = "Program log: Instruction: Create";

/// Returns true if this log notification represents a new token launch
pub fn is_new_token_launch(logs_value: &LogsValue) -> bool {
    // Skip failed transactions — no point scoring a failed mint
    if logs_value.err.is_some() {
        return false;
    }

    logs_value
        .logs
        .iter()
        .any(|log| log.contains(PUMPFUN_CREATE))
}
