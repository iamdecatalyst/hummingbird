import os

ORCHESTRATOR_URL = os.getenv("ORCHESTRATOR_URL", "http://localhost:8002")
RPC_HTTP = os.getenv("SOLANA_RPC", "https://api.mainnet-beta.solana.com")
PORT = int(os.getenv("SCORER_PORT", "8001"))

# Shared secret with the listener (incoming /score) and orchestrator (outgoing /trade).
# Required — startup will fail if unset. See orchestrator/main.go checkScorerAuth.
SCORER_SECRET = os.getenv("SCORER_SECRET", "")

# Cricket Protocol — Mantis (token risk) + Firefly (smart-money signals)
# Sign up at https://cricket.vylth.com
CRICKET_API_KEY = os.getenv("CRICKET_API_KEY", "")
CRICKET_BASE_URL = os.getenv("CRICKET_BASE_URL", "https://api-cricket.vylth.com")

# Position sizing — thresholds calibrated for Cricket+Firefly combined score (0-100).
# pump.fun pre-graduation tokens score structurally lower (LP not lockable yet),
# so thresholds are lower than the old RPC-only scorer.
SKIP_BELOW = 40
SMALL_BELOW = 55   # 40-54 → 0.05 SOL
MEDIUM_BELOW = 70  # 55-69 → 0.10 SOL
# 70+             → 0.20 SOL (full)

POSITION_SMALL  = 0.05
POSITION_MEDIUM = 0.10
POSITION_FULL   = 0.20
