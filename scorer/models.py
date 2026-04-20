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

    # Cricket metadata for rich Telegram broadcast
    rating: str = ""                             # low | moderate | high | critical
    mint_authority_revoked: Optional[bool] = None
    freeze_authority_revoked: Optional[bool] = None
    bonding_fill_pct: Optional[float] = None
    dev_supply_pct: Optional[float] = None
    top_10_holder_pct: Optional[float] = None
    deployer_wallet_age_days: Optional[int] = None
    deployer_prior_launches: Optional[int] = None
    firefly_score: Optional[int] = None
    firefly_win_rate: Optional[float] = None
    scan_flags: list[str] = []                   # high/critical flag detail strings
