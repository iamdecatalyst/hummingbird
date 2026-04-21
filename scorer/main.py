"""
Hummingbird Scorer — FastAPI service.

Two jobs:
  1. Receive TokenDetected from Rust listener → score for sniper entry → forward to orchestrator
  2. Run scalper background task → find second-wave patterns → forward scalp signals
"""
import asyncio
import hmac
import json
import logging
import os
import sys
import time
import uvicorn
from contextlib import asynccontextmanager

import httpx
from fastapi import BackgroundTasks, Depends, FastAPI, Header, HTTPException, status

from config import ORCHESTRATOR_URL, PORT, SCORER_SECRET
from models import ScoreResult, TokenDetected
from scalper import Scalper
from scorer import score as run_score
from store import TokenStore
from swing import Swing

logging.basicConfig(level=logging.INFO, format="%(asctime)s [scorer] %(message)s")
log = logging.getLogger(__name__)

# Shared state
store = TokenStore(max_age_minutes=45)
scalper = Scalper(store=store, orchestrator_url=ORCHESTRATOR_URL)
swing = Swing(orchestrator_url=ORCHESTRATOR_URL)


if not SCORER_SECRET or len(SCORER_SECRET) < 32:
    print(
        "[scorer] FATAL: SCORER_SECRET env var must be set (>=32 chars). "
        "Same value goes in orchestrator and listener .env files.",
        file=sys.stderr,
    )
    sys.exit(1)


def require_scorer_auth(authorization: str = Header(default="")) -> None:
    """Constant-time bearer-token check for listener → scorer endpoints."""
    prefix = "Bearer "
    if not authorization.startswith(prefix) or not hmac.compare_digest(
        authorization[len(prefix):], SCORER_SECRET
    ):
        raise HTTPException(status_code=status.HTTP_401_UNAUTHORIZED, detail="unauthorized")


@asynccontextmanager
async def lifespan(app: FastAPI):
    log.info("🐍 Hummingbird Scorer ready on port %d", PORT)
    log.info("   Orchestrator: %s", ORCHESTRATOR_URL)
    # Start scalper + swing background loops
    t1 = asyncio.create_task(scalper.run())
    t2 = asyncio.create_task(swing.run())
    yield
    t1.cancel()
    t2.cancel()


app = FastAPI(title="Hummingbird Scorer", lifespan=lifespan)


# ── Sniper endpoint ───────────────────────────────────────────────────────────

@app.post("/score", dependencies=[Depends(require_scorer_auth)])
async def score_token(token: TokenDetected, background_tasks: BackgroundTasks):
    """
    Receives a new token from the Rust listener.
    1. Adds it to the store (scalper will evaluate it in ~30s)
    2. Scores it immediately for sniper entry
    Both happen in the background so the listener isn't blocked.
    """
    # Always store — even if we skip sniper entry, scalper may catch a second wave
    store.add(
        mint=token.mint,
        platform=token.platform,
        chain=token.chain,
        dev_wallet=token.dev_wallet,
        bonding_curve=token.bonding_curve,
        timestamp_ms=token.timestamp_ms,
    )
    background_tasks.add_task(_score_and_forward, token)
    return {"status": "queued", "mint": token.mint}


async def _score_and_forward(token: TokenDetected):
    result = await run_score(token)

    mode = "sniper"
    log.info(
        "%s [%s] %s...  score=%d/100  decision=%s  position=%.2f SOL",
        "✅" if result.decision != "skip" else "⏭ ",
        mode,
        token.mint[:8],
        result.total,
        result.decision,
        result.position_sol,
    )
    _log_breakdown(result)
    result.engine = "sniper"
    _write_score_log(result, "sniper")

    await _forward(result)


# ── Scalper callback ──────────────────────────────────────────────────────────

@app.post("/scalper/closed")
async def scalper_position_closed(body: dict):
    mint = body.get("mint", "")
    if mint:
        scalper.on_position_closed(mint)
    return {"status": "ok"}


@app.post("/swing/closed")
async def swing_position_closed(body: dict):
    mint = body.get("mint", "")
    if mint:
        swing.on_position_closed(mint)
    return {"status": "ok"}


# ── Shared helpers ────────────────────────────────────────────────────────────

SCORE_LOG_PATH = os.environ.get("SCORE_LOG_PATH", "/opt/hummingbird/logs/scores.jsonl")

def _write_score_log(result: ScoreResult, source: str):
    """Append one JSON line per scored token for pattern analysis."""
    try:
        os.makedirs(os.path.dirname(SCORE_LOG_PATH), exist_ok=True)
        record = {
            "ts": int(time.time()),
            "source": source,
            "mint": result.mint,
            "platform": result.platform,
            "decision": result.decision,
            "total": result.total,
            "position_sol": result.position_sol,
            "rating": result.rating,
            "ai_summary": result.ai_summary,
            "checks": {k: {"score": v.score, "max": v.max_score, "reason": v.reason} for k, v in result.checks.items()},
            "flags": result.scan_flags,
            "mint_authority_revoked": result.mint_authority_revoked,
            "freeze_authority_revoked": result.freeze_authority_revoked,
            "dev_supply_pct": result.dev_supply_pct,
            "top_10_holder_pct": result.top_10_holder_pct,
            "deployer_wallet_age_days": result.deployer_wallet_age_days,
            "deployer_prior_launches": result.deployer_prior_launches,
        }
        with open(SCORE_LOG_PATH, "a") as f:
            f.write(json.dumps(record) + "\n")
    except Exception as e:
        log.warning("[scorelog] write failed: %s", e)


def _log_breakdown(result: ScoreResult):
    for name, check in result.checks.items():
        log.info("   %-14s %2d/%d  %s", name, check.score, check.max_score, check.reason)


async def _forward(result: ScoreResult):
    url = f"{ORCHESTRATOR_URL}/trade"
    headers = {"Authorization": f"Bearer {SCORER_SECRET}"}
    try:
        async with httpx.AsyncClient(timeout=3.0) as client:
            resp = await client.post(url, json=result.model_dump(), headers=headers)
            if resp.status_code == 200:
                log.info("   → forwarded to orchestrator")
            else:
                log.error("   → orchestrator returned %d", resp.status_code)
    except Exception as e:
        log.error("   → failed to reach orchestrator: %s", e)


# ── Status endpoints ──────────────────────────────────────────────────────────

@app.get("/health")
async def health():
    return {"status": "ok"}


@app.get("/store/stats")
async def store_stats():
    eligible = store.get_eligible()
    return {
        "total_tokens": store.size(),
        "eligible_for_scalp": len(eligible),
        "active_scalps": len(scalper._active),
    }


if __name__ == "__main__":
    uvicorn.run("main:app", host="0.0.0.0", port=PORT, log_level="warning")
