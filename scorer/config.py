import os

RPC_HTTP = os.getenv("RPC_HTTP", "https://api.mainnet-beta.solana.com")
ORCHESTRATOR_URL = os.getenv("ORCHESTRATOR_URL", "http://localhost:8002")
PORT = int(os.getenv("SCORER_PORT", "8001"))

# Shared secret with the listener (incoming /score) and orchestrator (outgoing /trade).
# Required — startup will fail if unset. See orchestrator/main.go checkScorerAuth.
SCORER_SECRET = os.getenv("SCORER_SECRET", "")

# pump.fun program
PUMP_FUN_PROGRAM = os.getenv(
    "PUMP_FUN_PROGRAM", "6EF8rrecthR5Dkzon8Nwu78hRvfCKubJ14M5uBEwF6P"
)

# Metaplex Token Metadata program (for fetching token name/socials)
TOKEN_METADATA_PROGRAM = "metaqbxxUerdq28cj1RbAWkYQm3ybzjb6a8bt518x1s"

# Score thresholds → position sizing
SKIP_BELOW = 60
SMALL_BELOW = 75   # 60-74 → 0.05 SOL
MEDIUM_BELOW = 90  # 75-89 → 0.10 SOL
# 90+             → 0.20 SOL (full)

POSITION_SMALL  = 0.05
POSITION_MEDIUM = 0.10
POSITION_FULL   = 0.20

# Bonding curve: pump.fun graduates at ~85 SOL
GRADUATION_SOL = 85.0
BONDING_SWEET_MIN = 0.05   # 5%  fill — too early = suspicious
BONDING_SWEET_MAX = 0.25   # 25% fill — past here is late entry
