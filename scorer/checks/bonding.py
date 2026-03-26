"""
Bonding curve check — 0 to 20 points.

pump.fun bonding curve account layout (after 8-byte discriminator):
  [8-15]   virtualTokenReserves  u64 LE
  [16-23]  virtualSolReserves    u64 LE
  [24-31]  realTokenReserves     u64 LE
  [32-39]  realSolReserves       u64 LE  ← used to estimate fill %
  [40-47]  tokenTotalSupply      u64 LE
  [48]     complete              bool    ← graduated to Raydium already?

Fill % = realSolReserves / GRADUATION_SOL
Sweet spot: 5–25% — early enough to ride the pump, late enough to confirm legitimacy.
"""
import base64
import struct
from rpc import get_account_info
from config import GRADUATION_SOL, BONDING_SWEET_MIN, BONDING_SWEET_MAX
from models import CheckResult

MAX_SCORE = 20
LAMPORTS_PER_SOL = 1_000_000_000


async def check(bonding_curve: str, platform: str = "pump_fun") -> CheckResult:
    # Only pump_fun uses the standard bonding curve byte layout.
    # Other platforms use different on-chain structures — return neutral score.
    if platform != "pump_fun":
        return CheckResult(score=10, max_score=MAX_SCORE, reason=f"{platform}: bonding curve check skipped (neutral)")

    try:
        account = await get_account_info(bonding_curve, encoding="base64")
        return _score(account)
    except Exception as e:
        return CheckResult(score=0, max_score=MAX_SCORE, reason=f"RPC error: {e}")


def _score(account: dict | None) -> CheckResult:
    if not account:
        return CheckResult(score=0, max_score=MAX_SCORE, reason="bonding curve not found")

    data_b64 = account.get("data")
    if not data_b64 or not isinstance(data_b64, list):
        return CheckResult(score=0, max_score=MAX_SCORE, reason="unexpected data format")

    try:
        data = base64.b64decode(data_b64[0])
    except Exception:
        return CheckResult(score=0, max_score=MAX_SCORE, reason="base64 decode failed")

    if len(data) < 49:
        return CheckResult(score=0, max_score=MAX_SCORE, reason="bonding curve data too short")

    # Parse fields (little-endian u64)
    real_sol_reserves = struct.unpack_from("<Q", data, 32)[0]
    complete = data[48] == 1

    if complete:
        # Already graduated to Raydium — this is no longer a bonding curve play
        return CheckResult(score=0, max_score=MAX_SCORE, reason="already graduated to Raydium")

    fill = (real_sol_reserves / LAMPORTS_PER_SOL) / GRADUATION_SOL
    fill_pct = fill * 100

    if fill < BONDING_SWEET_MIN:
        # Too early — almost nothing in the curve, could be a test or instant rug
        score = 5
        reason = f"very early ({fill_pct:.1f}% filled) — high risk"
    elif fill <= BONDING_SWEET_MAX:
        # Sweet spot
        score = MAX_SCORE
        reason = f"sweet spot ({fill_pct:.1f}% filled)"
    elif fill <= 0.50:
        # Getting late but still tradeable
        score = 12
        reason = f"past sweet spot ({fill_pct:.1f}% filled)"
    elif fill <= 0.75:
        # Late entry — upside shrinks
        score = 6
        reason = f"late entry ({fill_pct:.1f}% filled)"
    else:
        # Near graduation — minimal upside left
        score = 2
        reason = f"near graduation ({fill_pct:.1f}% filled)"

    return CheckResult(score=score, max_score=MAX_SCORE, reason=reason)
