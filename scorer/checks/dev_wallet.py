"""
Dev wallet check — 0 to 20 points.

Signals we look for:
  - Fresh wallet (created days ago) = high rug risk
  - Multiple prior token launches from same wallet = serial rugger pattern
  - Very low SOL balance = dev plans to dump and run
"""
import time
from rpc import get_account_info, get_signatures
from config import PUMP_FUN_PROGRAM
from models import CheckResult

MAX_SCORE = 20
LAMPORTS_PER_SOL = 1_000_000_000


async def check(dev_wallet: str) -> CheckResult:
    try:
        account, sigs = await _fetch(dev_wallet)
        return _score(dev_wallet, account, sigs)
    except Exception as e:
        # Never crash the pipeline — just give 0 if RPC fails
        return CheckResult(score=0, max_score=MAX_SCORE, reason=f"RPC error: {e}")


async def _fetch(dev_wallet: str):
    import asyncio
    account, sigs = await asyncio.gather(
        get_account_info(dev_wallet, encoding="jsonParsed"),
        get_signatures(dev_wallet, limit=50),
    )
    return account, sigs


def _score(dev_wallet: str, account: dict | None, sigs: list[dict]) -> CheckResult:
    if not account:
        return CheckResult(score=0, max_score=MAX_SCORE, reason="wallet not found")

    score = MAX_SCORE
    reasons = []

    # --- Balance check ---
    lamports = account.get("lamports", 0)
    sol_balance = lamports / LAMPORTS_PER_SOL

    if sol_balance < 0.1:
        score -= 8
        reasons.append(f"low balance ({sol_balance:.3f} SOL)")
    elif sol_balance < 0.5:
        score -= 3
        reasons.append(f"moderate balance ({sol_balance:.3f} SOL)")

    # --- Wallet age: estimate from oldest signature ---
    if sigs:
        oldest = sigs[-1]
        block_time = oldest.get("blockTime")
        if block_time:
            age_days = (time.time() - block_time) / 86400
            if age_days < 1:
                score -= 10
                reasons.append(f"brand new wallet ({age_days:.1f}d old)")
            elif age_days < 7:
                score -= 6
                reasons.append(f"very fresh wallet ({age_days:.1f}d old)")
            elif age_days < 30:
                score -= 2
                reasons.append(f"young wallet ({age_days:.0f}d old)")
    else:
        # No signatures = truly fresh wallet
        score -= 10
        reasons.append("no transaction history")

    # --- Prior pump.fun launches (serial rugger signal) ---
    pump_launches = sum(
        1 for s in sigs
        if s.get("memo") and PUMP_FUN_PROGRAM in s.get("memo", "")
    )
    # Rough heuristic: multiple pump.fun txs = has launched tokens before
    if pump_launches >= 3:
        score -= 5
        reasons.append(f"{pump_launches} prior pump.fun interactions")

    score = max(0, score)
    reason = "; ".join(reasons) if reasons else "wallet looks clean"
    return CheckResult(score=score, max_score=MAX_SCORE, reason=reason)
