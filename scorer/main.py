"""
Hummingbird Scorer — FastAPI service.

Two jobs:
  1. Receive TokenDetected from Rust listener → score for sniper entry → forward to orchestrator
  2. Run scalper background task → find second-wave patterns → forward scalp signals
"""
import asyncio
import logging
import uvicorn
from contextlib import asynccontextmanager

import httpx
from fastapi import BackgroundTasks, FastAPI

from config import ORCHESTRATOR_URL, PORT
from models import ScoreResult, TokenDetected
from scalper import Scalper
from scorer import score as run_score
from store import TokenStore

logging.basicConfig(level=logging.INFO, format="%(asctime)s [scorer] %(message)s")
log = logging.getLogger(__name__)

# Shared state
store = TokenStore(max_age_minutes=45)
scalper = Scalper(store=store, orchestrator_url=ORCHESTRATOR_URL)


@asynccontextmanager
async def lifespan(app: FastAPI):
    log.info("🐍 Hummingbird Scorer ready on port %d", PORT)
    log.info("   Orchestrator: %s", ORCHESTRATOR_URL)
    # Start scalper background loop
    task = asyncio.create_task(scalper.run())
    yield
    task.cancel()


app = FastAPI(title="Hummingbird Scorer", lifespan=lifespan)


# ── Sniper endpoint ───────────────────────────────────────────────────────────

@app.post("/score")
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

    if result.decision == "skip":
        return

    await _forward(result)


# ── Scalper callback ──────────────────────────────────────────────────────────

@app.post("/scalper/closed")
async def scalper_position_closed(body: dict):
    """
    Called by the orchestrator when a scalp position closes.
    Frees the slot so the same token can be scalped again if the pattern repeats.
    """
    mint = body.get("mint", "")
    if mint:
        scalper.on_position_closed(mint)
    return {"status": "ok"}


# ── Shared helpers ────────────────────────────────────────────────────────────

def _log_breakdown(result: ScoreResult):
    for name, check in result.checks.items():
        log.info("   %-14s %2d/%d  %s", name, check.score, check.max_score, check.reason)


async def _forward(result: ScoreResult):
    url = f"{ORCHESTRATOR_URL}/trade"
    try:
        async with httpx.AsyncClient(timeout=3.0) as client:
            resp = await client.post(url, json=result.model_dump())
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
