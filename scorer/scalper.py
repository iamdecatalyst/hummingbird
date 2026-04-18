"""
Scalper scanner — finds second-wave momentum on already-trading tokens.

Runs every 30 seconds as a background task.
Queries DexScreener for price/volume data on all eligible tokens.
Scores them for the second-wave pattern and forwards signals to the orchestrator.

Second-wave pattern:
  Token launched → pumped → pulled back 20-40% from ATH
  → volume returning → buyers outweigh sellers → enter for the second pump
"""
import asyncio
import logging
import time
from typing import Optional

import httpx

from config import SCORER_SECRET
from models import CheckResult, ScoreResult
from store import StoredToken, TokenStore

log = logging.getLogger(__name__)

DEXSCREENER_BASE = "https://api.dexscreener.com"
SCAN_INTERVAL_SECONDS = 30
BATCH_SIZE = 30  # DexScreener allows up to 30 tokens per request
ENTRY_THRESHOLD = 60  # minimum score to enter a scalp

# DexScreener chain ID mapping
CHAIN_MAP = {
    "solana": "solana",
    "base": "base",
    "bnb": "bsc",
}


class Scalper:
    def __init__(self, store: TokenStore, orchestrator_url: str):
        self.store = store
        self.orchestrator_url = orchestrator_url
        self._active: set[str] = set()  # mints currently in an open scalp position

    async def run(self):
        """Background loop — call with asyncio.create_task()."""
        log.info("🔍 Scalper scanner started (interval: %ds)", SCAN_INTERVAL_SECONDS)
        while True:
            await asyncio.sleep(SCAN_INTERVAL_SECONDS)
            try:
                await self._scan()
                self.store.cleanup()
            except Exception as e:
                log.error("[scalper] scan error: %s", e)

    # ── Main scan ─────────────────────────────────────────────────────────────

    async def _scan(self):
        eligible = self.store.get_eligible()
        if not eligible:
            return

        # Skip anything we're already in
        eligible = [t for t in eligible if t.mint not in self._active]
        if not eligible:
            return

        log.info("[scalper] scanning %d eligible tokens", len(eligible))

        # Group by DexScreener chain ID for batched requests
        by_chain: dict[str, list[StoredToken]] = {}
        for token in eligible:
            chain_id = CHAIN_MAP.get(token.chain, token.chain)
            by_chain.setdefault(chain_id, []).append(token)

        signals: list[ScoreResult] = []

        async with httpx.AsyncClient(timeout=10.0) as client:
            for chain_id, tokens in by_chain.items():
                for i in range(0, len(tokens), BATCH_SIZE):
                    batch = tokens[i : i + BATCH_SIZE]
                    pairs = await self._fetch_pairs(client, chain_id, batch)
                    for token, pair in zip(batch, pairs):
                        if pair is None:
                            continue
                        signal = self._score(token, pair)
                        if signal:
                            signals.append(signal)

        for signal in signals:
            await self._forward(signal)

    # ── DexScreener fetch ─────────────────────────────────────────────────────

    async def _fetch_pairs(
        self,
        client: httpx.AsyncClient,
        chain_id: str,
        tokens: list[StoredToken],
    ) -> list[Optional[dict]]:
        addresses = ",".join(t.mint for t in tokens)
        url = f"{DEXSCREENER_BASE}/tokens/v1/{chain_id}/{addresses}"

        try:
            resp = await client.get(url)
            resp.raise_for_status()
            data = resp.json()
            pairs_list = data if isinstance(data, list) else []

            # For each mint, keep the pair with highest liquidity
            pair_map: dict[str, dict] = {}
            for pair in pairs_list:
                addr = (pair.get("baseToken") or {}).get("address", "").lower()
                if not addr:
                    continue
                liq = float((pair.get("liquidity") or {}).get("usd") or 0)
                existing_liq = float(
                    (pair_map.get(addr, {}).get("liquidity") or {}).get("usd") or 0
                )
                if liq > existing_liq:
                    pair_map[addr] = pair

            return [pair_map.get(t.mint.lower()) for t in tokens]

        except Exception as e:
            log.error("[scalper] DexScreener error (%s): %s", chain_id, e)
            return [None] * len(tokens)

    # ── Scoring ───────────────────────────────────────────────────────────────

    def _score(self, token: StoredToken, pair: dict) -> Optional[ScoreResult]:
        """
        Score a token for scalp entry.
        Returns a ScoreResult if the second-wave pattern is strong enough.
        """
        now_ms = int(time.time() * 1000)
        age_minutes = (now_ms - token.first_seen_ms) / 60_000

        price_usd = float(pair.get("priceUsd") or 0)
        if price_usd <= 0:
            return None

        # Update peak tracking
        self.store.update_price(token.mint, price_usd)
        stored = self.store.get(token.mint)
        peak = stored.peak_price_usd if stored else price_usd

        price_change_5m = float((pair.get("priceChange") or {}).get("m5") or 0)
        vol_5m = float((pair.get("volume") or {}).get("m5") or 0)
        liquidity_usd = float((pair.get("liquidity") or {}).get("usd") or 0)

        txns_5m = (pair.get("txns") or {}).get("m5") or {}
        buys_5m = int(txns_5m.get("buys") or 0)
        sells_5m = int(txns_5m.get("sells") or 0)

        checks: dict[str, CheckResult] = {}
        total = 0

        # ── 1. Pullback depth — 25pts ─────────────────────────────────────────
        pullback_pct = ((peak - price_usd) / peak * 100) if peak > 0 else 0

        if 20 <= pullback_pct <= 40:
            pts, reason = 25, f"ideal pullback ({pullback_pct:.1f}% from ATH)"
        elif 15 <= pullback_pct < 20:
            pts, reason = 15, f"shallow pullback ({pullback_pct:.1f}% from ATH)"
        elif 40 < pullback_pct <= 55:
            pts, reason = 10, f"deep pullback ({pullback_pct:.1f}% from ATH)"
        else:
            pts, reason = 0, f"no pullback pattern ({pullback_pct:.1f}% from ATH)"

        checks["pullback"] = CheckResult(score=pts, max_score=25, reason=reason)
        total += pts

        # ── 2. Volume recovery — 25pts ────────────────────────────────────────
        if price_change_5m > 0 and vol_5m > 500:
            pts, reason = 25, f"volume + price recovering (+{price_change_5m:.1f}%, ${vol_5m:,.0f})"
        elif price_change_5m > 0:
            pts, reason = 15, f"price recovering, low volume (${vol_5m:,.0f})"
        elif vol_5m > 500:
            pts, reason = 10, f"volume present, price flat ({price_change_5m:.1f}%)"
        else:
            pts, reason = 0, "no volume recovery signal"

        checks["volume_recovery"] = CheckResult(score=pts, max_score=25, reason=reason)
        total += pts

        # ── 3. Buy pressure — 20pts ───────────────────────────────────────────
        total_txns = buys_5m + sells_5m
        if total_txns > 0:
            buy_ratio = buys_5m / total_txns
            if buy_ratio >= 0.65:
                pts, reason = 20, f"strong buyers ({buys_5m}B / {sells_5m}S)"
            elif buy_ratio >= 0.50:
                pts, reason = 12, f"neutral ({buys_5m}B / {sells_5m}S)"
            else:
                pts, reason = 0, f"sellers dominant ({buys_5m}B / {sells_5m}S)"
        else:
            pts, reason = 0, "no recent transactions"

        checks["buy_pressure"] = CheckResult(score=pts, max_score=20, reason=reason)
        total += pts

        # ── 4. Age sweet spot — 15pts ─────────────────────────────────────────
        if 8 <= age_minutes <= 25:
            pts, reason = 15, f"ideal age ({age_minutes:.1f}m)"
        elif 5 <= age_minutes < 8:
            pts, reason = 10, f"young ({age_minutes:.1f}m)"
        elif 25 < age_minutes <= 45:
            pts, reason = 8, f"maturing ({age_minutes:.1f}m)"
        else:
            pts, reason = 0, f"outside window ({age_minutes:.1f}m)"

        checks["age"] = CheckResult(score=pts, max_score=15, reason=reason)
        total += pts

        # ── 5. Liquidity — 15pts ──────────────────────────────────────────────
        if liquidity_usd >= 10_000:
            pts, reason = 15, f"good liquidity (${liquidity_usd:,.0f})"
        elif liquidity_usd >= 3_000:
            pts, reason = 8, f"moderate liquidity (${liquidity_usd:,.0f})"
        else:
            pts, reason = 0, f"low liquidity (${liquidity_usd:,.0f})"

        checks["liquidity"] = CheckResult(score=pts, max_score=15, reason=reason)
        total += pts

        # ── Decision ──────────────────────────────────────────────────────────
        if total < ENTRY_THRESHOLD:
            return None

        position_sol = 0.05 if total < 75 else 0.10

        log.info(
            "[scalper] 🎯 %s...  score=%d/100  age=%.1fm  pullback=%.1f%%  liq=$%.0f",
            token.mint[:8], total, age_minutes, pullback_pct, liquidity_usd,
        )
        self._log_breakdown(token.mint, checks)

        return ScoreResult(
            mint=token.mint,
            total=total,
            decision="scalp",
            position_sol=position_sol,
            checks=checks,
            scored_at_ms=now_ms,
        )

    def _log_breakdown(self, mint: str, checks: dict[str, CheckResult]):
        for name, c in checks.items():
            log.info("   %-18s %2d/%d  %s", name, c.score, c.max_score, c.reason)

    # ── Forward to orchestrator ───────────────────────────────────────────────

    async def _forward(self, signal: ScoreResult):
        url = f"{self.orchestrator_url}/trade"
        headers = {"Authorization": f"Bearer {SCORER_SECRET}"}
        try:
            async with httpx.AsyncClient(timeout=3.0) as client:
                resp = await client.post(url, json=signal.model_dump(), headers=headers)
                if resp.status_code == 200:
                    self._active.add(signal.mint)
                    log.info("[scalper] → forwarded %s to orchestrator", signal.mint[:8])
                else:
                    log.error("[scalper] orchestrator returned %d", resp.status_code)
        except Exception as e:
            log.error("[scalper] forward error: %s", e)

    def on_position_closed(self, mint: str):
        """Call this when orchestrator closes a scalp — frees the slot."""
        self._active.discard(mint)
