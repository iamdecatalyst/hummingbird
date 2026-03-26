from pydantic import BaseModel
from typing import Optional


class TokenDetected(BaseModel):
    mint: str
    signature: str
    dev_wallet: str
    bonding_curve: str
    timestamp_ms: int
    slot: int


class CheckResult(BaseModel):
    score: int      # points awarded for this check
    max_score: int  # max possible from this check
    reason: str     # human-readable explanation


class ScoreResult(BaseModel):
    mint: str
    total: int      # 0–100 final score
    decision: str   # "skip" | "small" | "medium" | "full"
    position_sol: float
    checks: dict[str, CheckResult]
    scored_at_ms: int
