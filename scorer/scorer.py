"""
Cricket-powered scorer.

Calls Mantis (token risk scan) + Firefly (dev wallet smart-money profile) concurrently,
then applies the same penalty/bonus logic as scoreFromCricket() in orchestrator/main.go.
"""
import asyncio
import logging
import time
from typing import Optional

import httpx

from config import (
    CRICKET_API_KEY, CRICKET_BASE_URL,
    ORCHESTRATOR_URL, SCORER_SECRET,
    SKIP_BELOW, SMALL_BELOW, MEDIUM_BELOW,
    POSITION_SMALL, POSITION_MEDIUM, POSITION_FULL,
)
from models import TokenDetected, ScoreResult, CheckResult

log = logging.getLogger(__name__)

_HEADERS = {
    "Authorization": f"Bearer {CRICKET_API_KEY}",
    "User-Agent": "hummingbird/1.0",
}


async def _mantis_scan(client: httpx.AsyncClient, mint: str, dev_wallet: str, bonding_curve: str) -> dict:
    url = f"{CRICKET_BASE_URL}/api/cricket/mantis/scan/{mint}"
    params: dict[str, str] = {}
    if dev_wallet:
        params["dev_wallet"] = dev_wallet
    if bonding_curve:
        params["bonding_curve"] = bonding_curve
    resp = await client.get(url, params=params)
    if resp.status_code in (404, 422):
        raise ValueError("token_not_found")
    resp.raise_for_status()
    return resp.json()


async def _firefly_wallet(client: httpx.AsyncClient, address: str) -> Optional[dict]:
    if not address:
        return None
    url = f"{CRICKET_BASE_URL}/api/cricket/firefly/wallet/{address}"
    resp = await client.get(url)
    if resp.status_code in (404, 422):
        return None
    resp.raise_for_status()
    return resp.json()


async def score(token: TokenDetected) -> ScoreResult:
    # Wait briefly for the launch tx to confirm before Mantis can see the mint account.
    await asyncio.sleep(1)

    async with httpx.AsyncClient(timeout=8.0, headers=_HEADERS) as client:
        mantis_task = asyncio.create_task(
            _mantis_scan(client, token.mint, token.dev_wallet or "", token.bonding_curve or "")
        )
        firefly_task = asyncio.create_task(
            _firefly_wallet(client, token.dev_wallet or "")
        )
        mantis_raw, firefly_raw = await asyncio.gather(mantis_task, firefly_task, return_exceptions=True)

    # If Mantis failed, skip immediately
    if isinstance(mantis_raw, Exception):
        reason = str(mantis_raw)
        if "token_not_found" in reason:
            reason = "token not found or not a valid mint"
        return ScoreResult(
            mint=token.mint,
            platform=token.platform,
            total=0,
            decision="skip",
            position_sol=0.0,
            checks={"mantis": CheckResult(score=0, max_score=100, reason=reason)},
            scored_at_ms=int(time.time() * 1000),
        )

    firefly_data = None if isinstance(firefly_raw, Exception) else firefly_raw

    total, decision, position_sol, checks = _apply_scoring(mantis_raw, firefly_data)

    # Extract raw fields for rich Telegram broadcast
    scan = mantis_raw.get("data", {}).get("scan", {})
    risk = mantis_raw.get("data", {}).get("risk_score", {})
    fw_data = (firefly_data or {}).get("data", {}) if firefly_data and firefly_data.get("success") else {}
    high_flags = [f["detail"] for f in scan.get("flags", []) if f.get("severity") in ("high", "critical") and f.get("detail")]
    ai_warning = (mantis_raw.get("data", {}).get("ai_analysis") or {}).get("warning")
    if ai_warning:
        high_flags.insert(0, f"🤖 {ai_warning}")

    return ScoreResult(
        mint=token.mint,
        platform=token.platform,
        total=total,
        decision=decision,
        position_sol=position_sol,
        checks=checks,
        scored_at_ms=int(time.time() * 1000),
        rating=risk.get("rating", ""),
        mint_authority_revoked=scan.get("mint_authority_revoked"),
        freeze_authority_revoked=scan.get("freeze_authority_revoked"),
        bonding_fill_pct=scan.get("bonding_curve_fill_pct"),
        dev_supply_pct=scan.get("dev_supply_pct"),
        top_10_holder_pct=scan.get("top_10_holder_pct") or None,
        deployer_wallet_age_days=scan.get("deployer_wallet_age_days"),
        deployer_prior_launches=scan.get("deployer_prior_launches"),
        firefly_score=fw_data.get("score") if fw_data else None,
        firefly_win_rate=fw_data.get("win_rate") if fw_data else None,
        scan_flags=high_flags,
    )


def _apply_scoring(mantis_raw: dict, firefly_raw: Optional[dict]) -> tuple[int, str, float, dict[str, CheckResult]]:
    data = mantis_raw.get("data", {})
    scan = data.get("scan", {})
    risk = data.get("risk_score", {})
    confidence = data.get("confidence", "none")

    # Mock data — score is meaningless
    if confidence != "high":
        return 0, "skip", 0.0, {
            "mantis": CheckResult(score=0, max_score=100, reason=f"low-confidence scan ({confidence})")
        }

    rating = risk.get("rating", "")
    if rating == "critical":
        return 0, "skip", 0.0, {
            "mantis": CheckResult(score=0, max_score=100, reason="critical risk rating")
        }

    # AI analysis — hunter+ tier only, absent for lower tiers (handle gracefully)
    ai = data.get("ai_analysis") or {}
    ai_intent = ai.get("intent", "")
    ai_delta = int(ai.get("ai_risk_delta") or 0)
    ai_confidence = ai.get("confidence", "low")

    if ai_intent == "likely_rug":
        return 0, "skip", 0.0, {
            "mantis": CheckResult(score=0, max_score=100, reason="AI: likely rug")
        }

    # Low liquidity hard skip
    flags = scan.get("flags", [])
    for f in flags:
        if f.get("check") == "low_liquidity" and f.get("severity") in ("high", "critical"):
            return 0, "skip", 0.0, {
                "mantis": CheckResult(score=0, max_score=100, reason="low_liquidity flag (high/critical)")
            }
    sol_reserve = scan.get("bonding_curve_sol_reserve")
    if sol_reserve is not None and sol_reserve < 0.5:
        return 0, "skip", 0.0, {
            "mantis": CheckResult(score=0, max_score=100, reason=f"bonding curve SOL reserve too low ({sol_reserve:.2f})")
        }

    base_score = risk.get("score", 50)
    score = base_score
    mantis_notes: list[str] = [f"base={base_score} ({rating})"]

    # Security flags
    if not scan.get("mint_authority_revoked", True):
        score -= 20
        mantis_notes.append("mint_authority_active -20")
    if not scan.get("freeze_authority_revoked", True):
        score -= 10
        mantis_notes.append("freeze_authority_active -10")
    if scan.get("metadata_mutable", False):
        score -= 5
        mantis_notes.append("metadata_mutable -5")

    # LP checks (only for graduated tokens)
    bonding_complete = scan.get("bonding_curve_complete")
    bonding_active = bonding_complete is None or not bonding_complete
    if not bonding_active:
        if scan.get("lp_locked", False):
            score += 10
            lock_days = scan.get("lp_lock_duration_days")
            if lock_days is not None and lock_days >= 30:
                score += 5
                mantis_notes.append("lp_locked≥30d +15")
            else:
                mantis_notes.append("lp_locked +10")
        else:
            score -= 10
            mantis_notes.append("lp_unlocked -10")

    # Bonding curve fill timing
    fill_pct = scan.get("bonding_curve_fill_pct")
    if fill_pct is not None:
        if 5 <= fill_pct <= 25:
            score += 10
            mantis_notes.append(f"fill={fill_pct:.1f}% sweet +10")
        elif 25 < fill_pct <= 60:
            score += 5
            mantis_notes.append(f"fill={fill_pct:.1f}% ok +5")
        elif fill_pct > 80:
            score -= 10
            mantis_notes.append(f"fill={fill_pct:.1f}% late -10")
        else:
            mantis_notes.append(f"fill={fill_pct:.1f}%")
    if bonding_complete:
        score -= 15
        mantis_notes.append("graduated -15")

    # Holder distribution
    top10 = scan.get("top_10_holder_pct", 0.0)
    if top10 > 80:
        score -= 20
        mantis_notes.append(f"top10={top10:.0f}% -20")
    elif top10 > 60:
        score -= 10
        mantis_notes.append(f"top10={top10:.0f}% -10")
    elif 0 < top10 < 30:
        score += 5
        mantis_notes.append(f"top10={top10:.0f}% +5")

    dev_pct = scan.get("dev_supply_pct")
    if dev_pct is not None:
        if dev_pct > 50:
            score -= 25
            mantis_notes.append(f"dev_supply={dev_pct:.0f}% -25")
        elif dev_pct > 30:
            score -= 15
            mantis_notes.append(f"dev_supply={dev_pct:.0f}% -15")
        elif dev_pct > 15:
            score -= 5
            mantis_notes.append(f"dev_supply={dev_pct:.0f}% -5")
        elif 0 < dev_pct < 5:
            score += 5
            mantis_notes.append(f"dev_supply={dev_pct:.0f}% +5")

    # Deployer history
    age_known = scan.get("deployer_age_known", False)
    age_days = scan.get("deployer_wallet_age_days", 0)
    if age_known:
        if age_days == 0:
            score -= 5
            mantis_notes.append("wallet_age=0d -5")
        elif age_days < 7:
            score -= 3
            mantis_notes.append(f"wallet_age={age_days}d -3")
        elif age_days > 90:
            score += 8
            mantis_notes.append(f"wallet_age={age_days}d +8")
        elif age_days > 30:
            score += 4
            mantis_notes.append(f"wallet_age={age_days}d +4")

    prior_launches = scan.get("deployer_prior_launches")
    if prior_launches is not None:
        if prior_launches > 10:
            score -= 10
            mantis_notes.append(f"prior_launches={prior_launches} -10")
        elif prior_launches > 3:
            score -= 5
            mantis_notes.append(f"prior_launches={prior_launches} -5")

    # Apply AI adjustments last so they layer on top of all rule-based scoring
    if ai_intent == "suspicious":
        score -= 15
        mantis_notes.append("AI:suspicious -15")
    if ai_confidence in ("medium", "high") and ai_delta != 0:
        score += ai_delta
        mantis_notes.append(f"AI:{ai_delta:+d}")

    mantis_score = max(0, min(100, score))
    checks: dict[str, CheckResult] = {
        "mantis": CheckResult(
            score=mantis_score,
            max_score=100,
            reason=", ".join(mantis_notes),
        )
    }

    # Firefly smart-money overlay
    firefly_delta = 0
    firefly_notes: list[str] = []
    if firefly_raw and firefly_raw.get("success"):
        dw = firefly_raw.get("data", {})
        fw_score = dw.get("score", 0)
        style = dw.get("style", "")
        win_rate = dw.get("win_rate", 0.0)
        avg_return = dw.get("avg_return_pct", 0.0)
        total_trades = dw.get("total_trades", 0)

        if fw_score >= 70:
            firefly_delta += 8
            firefly_notes.append(f"score={fw_score} +8")
        elif fw_score >= 50:
            firefly_delta += 3
            firefly_notes.append(f"score={fw_score} +3")
        elif fw_score < 10:
            firefly_delta -= 25
            firefly_notes.append(f"score={fw_score} -25")
        elif fw_score < 20:
            firefly_delta -= 15
            firefly_notes.append(f"score={fw_score} -15")
        else:
            firefly_notes.append(f"score={fw_score}")

        if style == "smart_contract_deployer" and win_rate > 75:
            firefly_delta -= 20
            firefly_notes.append(f"serial_deployer win_rate={win_rate:.0f}% -20")

        if avg_return > 100 and total_trades > 5:
            firefly_delta += 5
            firefly_notes.append(f"avg_return={avg_return:.0f}% +5")

        checks["firefly"] = CheckResult(
            score=max(0, firefly_delta + 25),  # offset so display isn't confusing
            max_score=50,
            reason=", ".join(firefly_notes) if firefly_notes else "no signal",
        )
    else:
        checks["firefly"] = CheckResult(score=0, max_score=0, reason="unavailable")

    total = max(0, min(100, mantis_score + firefly_delta))

    # Calibrated thresholds — pump.fun scores structurally low pre-graduation
    if total < 40:
        decision, pos = "skip", 0.0
    elif total < 55:
        decision, pos = "small", POSITION_SMALL
    elif total < 70:
        decision, pos = "medium", POSITION_MEDIUM
    else:
        decision, pos = "full", POSITION_FULL

    return total, decision, pos, checks
