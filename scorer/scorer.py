"""
Main scoring orchestrator.

All checks run concurrently via asyncio.gather — no sequential waiting.
Total latency = slowest individual check, not the sum of all checks.

Score breakdown (100 pts total):
  dev_wallet   20 pts  — age, history, balance
  supply       20 pts  — dev % of total supply
  bonding      20 pts  — curve fill %, not yet graduated
  contract     15 pts  — mint/freeze authority flags
  social       10 pts  — Twitter + Telegram presence
  ─────────────────────
  total        85 pts  (remaining 15 are reserved for future checks)
"""
import asyncio
import time
from models import TokenDetected, ScoreResult, CheckResult
from config import SKIP_BELOW, SMALL_BELOW, MEDIUM_BELOW
from config import POSITION_SMALL, POSITION_MEDIUM, POSITION_FULL
from checks import dev_wallet, supply, bonding, contract, social


async def score(token: TokenDetected) -> ScoreResult:
    # Run all checks simultaneously — speed is everything
    results = await asyncio.gather(
        dev_wallet.check(token.dev_wallet),
        supply.check(token.mint, token.dev_wallet),
        bonding.check(token.bonding_curve, token.platform),
        contract.check(token.mint, token.bonding_curve),
        social.check(token.mint),
        return_exceptions=True,
    )

    checks: dict[str, CheckResult] = {}
    total = 0

    labels = ["dev_wallet", "supply", "bonding", "contract", "social"]
    for label, result in zip(labels, results):
        if isinstance(result, Exception):
            checks[label] = CheckResult(score=0, max_score=_max_for(label), reason=str(result))
        else:
            checks[label] = result
            total += result.score

    decision, position_sol = _decide(total)

    return ScoreResult(
        mint=token.mint,
        total=total,
        decision=decision,
        position_sol=position_sol,
        checks=checks,
        scored_at_ms=int(time.time() * 1000),
    )


def _decide(total: int) -> tuple[str, float]:
    if total < SKIP_BELOW:
        return "skip", 0.0
    elif total < SMALL_BELOW:
        return "small", POSITION_SMALL
    elif total < MEDIUM_BELOW:
        return "medium", POSITION_MEDIUM
    else:
        return "full", POSITION_FULL


def _max_for(label: str) -> int:
    return {"dev_wallet": 20, "supply": 20, "bonding": 20, "contract": 15, "social": 10}.get(
        label, 10
    )
