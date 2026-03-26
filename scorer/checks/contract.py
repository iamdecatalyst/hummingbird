"""
Contract flags check — 0 to 15 points.

Solana mint account layout (82 bytes):
  [0]     mint authority option  (0 = None, 1 = Some)
  [1-32]  mint authority pubkey
  [33-40] supply (u64 LE)
  [41]    decimals
  [42]    is_initialized
  [43]    freeze authority option (0 = None, 1 = Some)
  [44-75] freeze authority pubkey

On pump.fun, mint authority is held by the bonding curve until graduation.
After graduation it's revoked. A non-pump mint authority = red flag.
Freeze authority present at all = red flag.
"""
import base64
import struct
from rpc import get_account_info
from config import PUMP_FUN_PROGRAM
from models import CheckResult

MAX_SCORE = 15
MINT_LAYOUT_SIZE = 82


async def check(mint: str, bonding_curve: str) -> CheckResult:
    try:
        account = await get_account_info(mint, encoding="base64")
        return _score(account, bonding_curve)
    except Exception as e:
        return CheckResult(score=0, max_score=MAX_SCORE, reason=f"RPC error: {e}")


def _score(account: dict | None, bonding_curve: str) -> CheckResult:
    if not account:
        return CheckResult(score=0, max_score=MAX_SCORE, reason="mint account not found")

    data_b64 = account.get("data")
    if not data_b64 or not isinstance(data_b64, list):
        return CheckResult(score=0, max_score=MAX_SCORE, reason="unexpected data format")

    try:
        data = base64.b64decode(data_b64[0])
    except Exception:
        return CheckResult(score=0, max_score=MAX_SCORE, reason="base64 decode failed")

    if len(data) < MINT_LAYOUT_SIZE:
        return CheckResult(score=0, max_score=MAX_SCORE, reason="mint data too short")

    score = MAX_SCORE
    reasons = []

    has_mint_auth = data[0] == 1
    has_freeze_auth = data[43] == 1

    if has_mint_auth:
        mint_auth = _parse_pubkey(data[1:33])
        # Acceptable: mint authority is the bonding curve (pump.fun standard)
        if mint_auth != bonding_curve:
            score -= 10
            reasons.append(f"non-standard mint authority ({mint_auth[:8]}...)")
        else:
            reasons.append("mint auth is bonding curve (normal)")
    else:
        reasons.append("mint authority revoked (good)")

    if has_freeze_auth:
        score -= 8
        freeze_auth = _parse_pubkey(data[44:76])
        reasons.append(f"freeze authority present ({freeze_auth[:8]}...)")
    else:
        reasons.append("no freeze authority (good)")

    score = max(0, score)
    return CheckResult(score=score, max_score=MAX_SCORE, reason="; ".join(reasons))


def _parse_pubkey(raw: bytes) -> str:
    """Base58-encode 32 raw bytes into a Solana pubkey string."""
    import base58  # type: ignore
    return base58.b58encode(raw).decode()
