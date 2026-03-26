"""
Dev supply concentration check — 0 to 20 points.

Checks what % of total supply the dev wallet holds.
>20% = major red flag (one wallet can nuke the price instantly).
"""
from rpc import rpc
from models import CheckResult

MAX_SCORE = 20


async def check(mint: str, dev_wallet: str) -> CheckResult:
    try:
        total, dev_balance = await _fetch(mint, dev_wallet)
        return _score(total, dev_balance)
    except Exception as e:
        return CheckResult(score=0, max_score=MAX_SCORE, reason=f"RPC error: {e}")


async def _fetch(mint: str, dev_wallet: str) -> tuple[int, int]:
    import asyncio

    async def get_total_supply() -> int:
        data = await rpc("getTokenSupply", [mint])
        amount = data.get("result", {}).get("value", {}).get("amount", "0")
        return int(amount)

    async def get_dev_balance() -> int:
        # getTokenAccountsByOwner → find the ATA for this mint
        data = await rpc(
            "getTokenAccountsByOwner",
            [
                dev_wallet,
                {"mint": mint},
                {"encoding": "jsonParsed"},
            ],
        )
        accounts = data.get("result", {}).get("value", [])
        if not accounts:
            return 0
        amount = (
            accounts[0]
            .get("account", {})
            .get("data", {})
            .get("parsed", {})
            .get("info", {})
            .get("tokenAmount", {})
            .get("amount", "0")
        )
        return int(amount)

    total, dev_bal = await asyncio.gather(get_total_supply(), get_dev_balance())
    return total, dev_bal


def _score(total: int, dev_balance: int) -> CheckResult:
    if total == 0:
        return CheckResult(score=0, max_score=MAX_SCORE, reason="supply is 0")

    dev_pct = (dev_balance / total) * 100

    if dev_pct > 50:
        score = 0
        reason = f"dev holds {dev_pct:.1f}% — extreme concentration"
    elif dev_pct > 20:
        score = 4
        reason = f"dev holds {dev_pct:.1f}% — high concentration"
    elif dev_pct > 10:
        score = 12
        reason = f"dev holds {dev_pct:.1f}% — moderate concentration"
    elif dev_pct > 5:
        score = 17
        reason = f"dev holds {dev_pct:.1f}% — acceptable"
    else:
        score = MAX_SCORE
        reason = f"dev holds {dev_pct:.1f}% — well distributed"

    return CheckResult(score=score, max_score=MAX_SCORE, reason=reason)
