from pydantic import BaseModel
from typing import Optional


class TokenDetected(BaseModel):
    mint: str
    signature: str
    dev_wallet: str
    bonding_curve: str
    timestamp_ms: int
    slot: int
    platform: str = "pump_fun"   # "pump_fun" | "moonshot" | "four_meme" | "virtuals" etc.
    chain: str = "solana"        # "solana" | "base" | "bnb"


class CheckResult(BaseModel):
    score: int
    max_score: int
    reason: str


class ScoreResult(BaseModel):
    mint: str
    total: int
    decision: str        # "skip" | "small" | "medium" | "full" | "scalp"
    position_sol: float
    checks: dict[str, CheckResult]
    scored_at_ms: int
