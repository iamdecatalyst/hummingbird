"""
Hummingbird Scorer — FastAPI service.

Receives TokenDetected from the Rust listener,
scores it, and forwards the ScoreResult to the Go orchestrator.
"""
import httpx
import uvicorn
from fastapi import FastAPI, BackgroundTasks
from contextlib import asynccontextmanager

from models import TokenDetected, ScoreResult
from scorer import score as run_score
from config import ORCHESTRATOR_URL, PORT

import logging
logging.basicConfig(level=logging.INFO, format="%(asctime)s [scorer] %(message)s")
log = logging.getLogger(__name__)


@asynccontextmanager
async def lifespan(app: FastAPI):
    log.info(f"🐍 Hummingbird Scorer ready on port {PORT}")
    log.info(f"   Forwarding to orchestrator: {ORCHESTRATOR_URL}")
    yield


app = FastAPI(title="Hummingbird Scorer", lifespan=lifespan)


@app.post("/score")
async def score_token(token: TokenDetected, background_tasks: BackgroundTasks):
    """
    Receives a token from the Rust listener.
    Scores it in the background so the listener isn't blocked waiting.
    """
    background_tasks.add_task(_score_and_forward, token)
    return {"status": "queued", "mint": token.mint}


async def _score_and_forward(token: TokenDetected):
    result = await run_score(token)

    log.info(
        f"{'✅' if result.decision != 'skip' else '⏭ '} "
        f"{token.mint[:8]}... "
        f"score={result.total}/100 "
        f"decision={result.decision} "
        f"position={result.position_sol} SOL"
    )
    _log_breakdown(result)

    if result.decision == "skip":
        return

    await _forward(result)


def _log_breakdown(result: ScoreResult):
    for name, check in result.checks.items():
        log.info(f"   {name:12s} {check.score:2d}/{check.max_score}  {check.reason}")


async def _forward(result: ScoreResult):
    url = f"{ORCHESTRATOR_URL}/trade"
    try:
        async with httpx.AsyncClient(timeout=3.0) as client:
            resp = await client.post(url, json=result.model_dump())
            if resp.status_code == 200:
                log.info(f"   → forwarded to orchestrator")
            else:
                log.error(f"   → orchestrator returned {resp.status_code}")
    except Exception as e:
        log.error(f"   → failed to reach orchestrator: {e}")


@app.get("/health")
async def health():
    return {"status": "ok"}


if __name__ == "__main__":
    uvicorn.run("main:app", host="0.0.0.0", port=PORT, log_level="warning")
