"""
Scalper — two-source discovery with Cricket + DexScreener confluence.

Discovery sources (run every 60s):
  1. Sniper store       — tokens the Rust listener already detected
  2. DexScreener boosts — tokens getting paid promotion (high activity)
  3. DexScreener profiles — recently listed tokens with social presence

Scoring:
  Step 1 — DexScreener pattern score (pullback, volume, buy pressure, age, liquidity)
  Step 2 — Cricket Mantis risk scan (gated: only run if DexScreener ≥ 40)
  Step 3 — Combined: DexScreener score + Cricket adjustment ≥ 60 → enter

Firefly signals endpoint has been unreliable; Cricket Mantis is used instead.
"""
import asyncio
import logging
import time
from typing import Optional

import httpx

from config import CRICKET_API_KEY, CRICKET_BASE_URL, SCORER_SECRET
from models import CheckResult, ScoreResult
from store import StoredToken, TokenStore

log = logging.getLogger(__name__)

DEXSCREENER_BASE = "https://api.dexscreener.com"
SCAN_INTERVAL_SECONDS = 30
DISCOVER_INTERVAL_SECONDS = 60
BATCH_SIZE = 30
DEXSCREENER_THRESHOLD = 40   # min DexScreener score before calling Cricket
ENTRY_THRESHOLD = 60          # min combined score to enter

CHAIN_MAP = {"solana": "solana", "base": "base", "bnb": "bsc"}

_CRICKET_HEADERS = {"Authorization": f"Bearer {CRICKET_API_KEY}", "User-Agent": "hummingbird/1.0"}


class Scalper:
    def __init__(self, store: TokenStore, orchestrator_url: str):
        self.store = store
        self.orchestrator_url = orchestrator_url
        self._active: set[str] = set()

    async def run(self):
        log.info("🔍 Scalper scanner started (interval: %ds)", SCAN_INTERVAL_SECONDS)
        discover_counter = 0
        while True:
            await asyncio.sleep(SCAN_INTERVAL_SECONDS)
            try:
                # Discovery runs every 2 scan cycles (~60s)
                discover_counter += 1
                if discover_counter >= 2:
                    discover_counter = 0
                    await self._discover_tokens()

                await self._scan()
                self.store.cleanup()
            except Exception as e:
                log.error("[scalper] scan error: %s", e)

    # ── DexScreener discovery ─────────────────────────────────────────────────

    async def _discover_tokens(self):
        """Pull Solana tokens from DexScreener boosts + profiles, add new ones to store."""
        mints: set[str] = set()

        async with httpx.AsyncClient(timeout=10.0) as client:
            for endpoint in ["/token-boosts/top/v1", "/token-profiles/latest/v1"]:
                try:
                    resp = await client.get(f"{DEXSCREENER_BASE}{endpoint}")
                    if resp.status_code != 200:
                        continue
                    items = resp.json()
                    if not isinstance(items, list):
                        continue
                    for item in items:
                        if item.get("chainId") != "solana":
                            continue
                        mint = item.get("tokenAddress")
                        if mint and not self.store.get(mint):
                            mints.add(mint)
                except Exception as e:
                    log.warning("[scalper] discovery %s error: %s", endpoint, e)

            if not mints:
                return

            # Fetch pair data to get age + liquidity before storing
            mint_list = list(mints)
            for i in range(0, len(mint_list), BATCH_SIZE):
                batch = mint_list[i:i + BATCH_SIZE]
                await self._store_discovered(client, batch)

    async def _store_discovered(self, client: httpx.AsyncClient, mints: list[str]):
        try:
            url = f"{DEXSCREENER_BASE}/tokens/v1/solana/{','.join(mints)}"
            resp = await client.get(url)
            if resp.status_code != 200:
                return
            pairs = resp.json()
            if not isinstance(pairs, list):
                return

            now_ms = int(time.time() * 1000)
            for pair in pairs:
                mint = (pair.get("baseToken") or {}).get("address")
                if not mint or self.store.get(mint):
                    continue
                liquidity = float((pair.get("liquidity") or {}).get("usd") or 0)
                if liquidity < 5_000:
                    continue
                created_at = pair.get("pairCreatedAt") or now_ms
                age_minutes = (now_ms - created_at) / 60_000
                # 10 min floor — avoids instant rug bots on brand-new tokens.
                # 4 hour cap — older tokens have weaker momentum and higher rug exposure.
                if age_minutes < 10 or age_minutes > 240:
                    continue
                self.store.add(
                    mint=mint,
                    platform="pump_fun",
                    chain="solana",
                    dev_wallet="",
                    bonding_curve="",
                    timestamp_ms=now_ms,
                )
                log.info("[scalper] 🔎 discovered %s... age=%.0fd%.0fh liq=$%.0f", mint[:8], age_minutes // 1440, (age_minutes % 1440) // 60, liquidity)
        except Exception as e:
            log.warning("[scalper] store_discovered error: %s", e)

    # ── Main scan ─────────────────────────────────────────────────────────────

    async def _scan(self):
        eligible = [t for t in self.store.get_eligible(min_age_minutes=0) if t.mint not in self._active]
        if not eligible:
            return

        log.info("[scalper] scanning %d eligible tokens", len(eligible))

        by_chain: dict[str, list[StoredToken]] = {}
        for token in eligible:
            chain_id = CHAIN_MAP.get(token.chain, token.chain)
            by_chain.setdefault(chain_id, []).append(token)

        candidates: list[tuple[StoredToken, dict, ScoreResult]] = []

        async with httpx.AsyncClient(timeout=10.0) as client:
            for chain_id, tokens in by_chain.items():
                for i in range(0, len(tokens), BATCH_SIZE):
                    batch = tokens[i:i + BATCH_SIZE]
                    pairs = await self._fetch_pairs(client, chain_id, batch)
                    for token, pair in zip(batch, pairs):
                        if pair is None:
                            continue
                        result = self._dex_score(token, pair)
                        if result and result.total >= DEXSCREENER_THRESHOLD:
                            candidates.append((token, pair, result))

            # Cricket validate candidates (concurrently, max 3 at a time)
            sem = asyncio.Semaphore(3)
            async with httpx.AsyncClient(timeout=6.0, headers=_CRICKET_HEADERS) as cc:
                tasks = [self._validate_and_forward(cc, sem, token, result) for token, _, result in candidates]
                await asyncio.gather(*tasks, return_exceptions=True)

    # ── DexScreener fetch ─────────────────────────────────────────────────────

    async def _fetch_pairs(self, client: httpx.AsyncClient, chain_id: str, tokens: list[StoredToken]) -> list[Optional[dict]]:
        addresses = ",".join(t.mint for t in tokens)
        url = f"{DEXSCREENER_BASE}/tokens/v1/{chain_id}/{addresses}"
        try:
            resp = await client.get(url)
            resp.raise_for_status()
            data = resp.json()
            pairs_list = data if isinstance(data, list) else []
            pair_map: dict[str, dict] = {}
            for pair in pairs_list:
                addr = (pair.get("baseToken") or {}).get("address", "").lower()
                if not addr:
                    continue
                liq = float((pair.get("liquidity") or {}).get("usd") or 0)
                if liq > float((pair_map.get(addr, {}).get("liquidity") or {}).get("usd") or 0):
                    pair_map[addr] = pair
            return [pair_map.get(t.mint.lower()) for t in tokens]
        except Exception as e:
            log.error("[scalper] DexScreener error (%s): %s", chain_id, e)
            return [None] * len(tokens)

    # ── DexScreener pattern scoring ───────────────────────────────────────────

    def _dex_score(self, token: StoredToken, pair: dict) -> Optional[ScoreResult]:
        now_ms = int(time.time() * 1000)

        price_usd = float(pair.get("priceUsd") or 0)
        if price_usd <= 0:
            return None

        self.store.update_price(token.mint, price_usd)
        stored = self.store.get(token.mint)
        peak = stored.peak_price_usd if stored else price_usd

        price_change_5m = float((pair.get("priceChange") or {}).get("m5") or 0)
        price_change_1h = float((pair.get("priceChange") or {}).get("h1") or 0)
        vol_5m = float((pair.get("volume") or {}).get("m5") or 0)
        vol_1h = float((pair.get("volume") or {}).get("h1") or 0)
        liquidity_usd = float((pair.get("liquidity") or {}).get("usd") or 0)
        txns_5m = (pair.get("txns") or {}).get("m5") or {}
        buys_5m = int(txns_5m.get("buys") or 0)
        sells_5m = int(txns_5m.get("sells") or 0)

        checks: dict[str, CheckResult] = {}
        total = 0

        # Pullback depth — 25pts (still useful: we track peak from repeated scans)
        pullback_pct = ((peak - price_usd) / peak * 100) if peak > 0 else 0
        if 20 <= pullback_pct <= 40:
            pts, reason = 25, f"ideal pullback ({pullback_pct:.1f}% from ATH)"
        elif 15 <= pullback_pct < 20:
            pts, reason = 15, f"shallow pullback ({pullback_pct:.1f}%)"
        elif 40 < pullback_pct <= 55:
            pts, reason = 10, f"deep pullback ({pullback_pct:.1f}%)"
        else:
            pts, reason = 0, f"no pullback ({pullback_pct:.1f}%)"
        checks["pullback"] = CheckResult(score=pts, max_score=25, reason=reason)
        total += pts

        # Volume momentum — 25pts (5m price up + meaningful volume)
        if price_change_5m > 0 and vol_5m > 500:
            pts, reason = 25, f"momentum +{price_change_5m:.1f}% ${vol_5m:,.0f}"
        elif price_change_5m > 0:
            pts, reason = 15, f"price up, thin vol ${vol_5m:,.0f}"
        elif vol_5m > 500:
            pts, reason = 10, f"vol active, flat ${vol_5m:,.0f}"
        else:
            pts, reason = 0, "no momentum"
        checks["momentum"] = CheckResult(score=pts, max_score=25, reason=reason)
        total += pts

        # Buy pressure — 20pts
        total_txns = buys_5m + sells_5m
        if total_txns > 0:
            buy_ratio = buys_5m / total_txns
            if buy_ratio >= 0.65:
                pts, reason = 20, f"strong buyers {buys_5m}B/{sells_5m}S"
            elif buy_ratio >= 0.50:
                pts, reason = 12, f"neutral {buys_5m}B/{sells_5m}S"
            else:
                pts, reason = 0, f"sellers dominate {buys_5m}B/{sells_5m}S"
        else:
            pts, reason = 0, "no txns"
        checks["buy_pressure"] = CheckResult(score=pts, max_score=20, reason=reason)
        total += pts

        # 1h trend — 20pts (replaces age — is the wave still going?)
        if price_change_1h > 20 and vol_1h > 5_000:
            pts, reason = 20, f"strong 1h trend +{price_change_1h:.0f}% ${vol_1h:,.0f}"
        elif price_change_1h > 5:
            pts, reason = 12, f"positive 1h +{price_change_1h:.0f}%"
        elif price_change_1h > 0:
            pts, reason = 6, f"flat 1h +{price_change_1h:.0f}%"
        elif price_change_1h > -10:
            pts, reason = 3, f"slight dip 1h {price_change_1h:.0f}%"
        else:
            pts, reason = 0, f"dumping 1h {price_change_1h:.0f}%"
        checks["trend_1h"] = CheckResult(score=pts, max_score=20, reason=reason)
        total += pts

        # Liquidity — 10pts (tightened: enough to enter/exit cleanly)
        if liquidity_usd >= 50_000:
            pts, reason = 10, f"deep ${liquidity_usd:,.0f}"
        elif liquidity_usd >= 10_000:
            pts, reason = 7, f"${liquidity_usd:,.0f}"
        elif liquidity_usd >= 3_000:
            pts, reason = 4, f"thin ${liquidity_usd:,.0f}"
        else:
            pts, reason = 0, f"low ${liquidity_usd:,.0f}"
        checks["liquidity"] = CheckResult(score=pts, max_score=10, reason=reason)
        total += pts

        return ScoreResult(
            mint=token.mint,
            platform=token.platform or "pump_fun",
            total=total,
            decision="scalp",
            position_sol=0.0,  # set after Cricket
            checks=checks,
            scored_at_ms=now_ms,
        )

    # ── Cricket validation ────────────────────────────────────────────────────

    async def _validate_and_forward(
        self,
        client: httpx.AsyncClient,
        sem: asyncio.Semaphore,
        token: StoredToken,
        dex_result: ScoreResult,
    ):
        async with sem:
            cricket_delta, cricket_check, meta = await self._cricket_scan(client, token)
            dex_result.checks["cricket"] = cricket_check

            # Hard skip if Cricket says critical
            if cricket_delta <= -100:
                log.info("[scalper] ❌ %s... cricket veto: %s", token.mint[:8], cricket_check.reason)
                return

            combined = max(0, min(100, dex_result.total + cricket_delta))
            dex_result.total = combined

            if combined < ENTRY_THRESHOLD:
                log.info("[scalper] ⏭  %s... combined=%d (dex+cricket) — below threshold", token.mint[:8], combined)
                return

            dex_result.position_sol = 0.05 if combined < 75 else 0.10
            dex_result.decision = "scalp"

            # Populate Cricket metadata for rich Telegram broadcast
            if meta:
                dex_result.rating = meta.get("rating", "")
                dex_result.mint_authority_revoked = meta.get("mint_authority_revoked")
                dex_result.freeze_authority_revoked = meta.get("freeze_authority_revoked")
                dex_result.bonding_fill_pct = meta.get("bonding_fill_pct")
                dex_result.dev_supply_pct = meta.get("dev_supply_pct")
                dex_result.top_10_holder_pct = meta.get("top_10_holder_pct")
                dex_result.deployer_wallet_age_days = meta.get("deployer_wallet_age_days")
                dex_result.deployer_prior_launches = meta.get("deployer_prior_launches")
                dex_result.scan_flags = meta.get("scan_flags", [])

            log.info(
                "[scalper] 🎯 %s...  combined=%d/100  dex=%d  cricket_adj=%+d  pos=%.2f SOL",
                token.mint[:8], combined, dex_result.total - cricket_delta,
                cricket_delta, dex_result.position_sol,
            )
            for name, c in dex_result.checks.items():
                log.info("   %-14s %2d/%d  %s", name, c.score, c.max_score, c.reason)

            await self._forward(dex_result)

    async def _cricket_scan(self, client: httpx.AsyncClient, token: StoredToken) -> tuple[int, CheckResult, dict]:
        """Returns (score_delta, CheckResult). delta is added to dex score."""
        if not CRICKET_API_KEY:
            return 0, CheckResult(score=0, max_score=0, reason="no API key"), {}

        try:
            params: dict[str, str] = {}
            if token.dev_wallet:
                params["dev_wallet"] = token.dev_wallet
            if token.bonding_curve:
                params["bonding_curve"] = token.bonding_curve

            resp = await client.get(
                f"{CRICKET_BASE_URL}/api/cricket/mantis/scan/{token.mint}",
                params=params,
            )
            if resp.status_code in (404, 422):
                return -10, CheckResult(score=0, max_score=30, reason="not found on-chain"), {}
            if resp.status_code != 200:
                return 0, CheckResult(score=15, max_score=30, reason=f"unavailable ({resp.status_code})"), {}

            data = resp.json().get("data", {})
            risk = data.get("risk_score", {})
            confidence = data.get("confidence", "none")
            rating = risk.get("rating", "unknown")
            base = risk.get("score", 50)

            if confidence != "high":
                return 0, CheckResult(score=15, max_score=30, reason=f"low confidence ({confidence})"), {}

            if rating == "critical":
                return -100, CheckResult(score=0, max_score=30, reason="CRITICAL — veto"), {}
            elif rating == "high":
                delta, note = -15, f"high risk ({base})"
            elif rating == "moderate":
                delta, note = 0, f"moderate ({base})"
            elif rating == "low":
                delta, note = +15, f"low risk ({base})"
            else:
                delta, note = 0, f"unknown ({base})"

            scan = data.get("scan", {})
            if not scan.get("mint_authority_revoked", True):
                delta -= 5
                note += " mint_active"
            if not scan.get("freeze_authority_revoked", True):
                delta -= 5
                note += " freeze_active"

            # AI analysis — hunter+ tier only, absent for lower tiers (handle gracefully)
            ai = data.get("ai_analysis") or {}
            ai_intent = ai.get("intent", "")
            ai_delta = int(ai.get("ai_risk_delta") or 0)
            ai_confidence = ai.get("confidence", "low")
            ai_warning = (ai.get("warning") or "").strip()

            if ai_intent == "likely_rug":
                return -100, CheckResult(score=0, max_score=30, reason="AI: likely rug — veto"), {}
            if ai_intent == "suspicious":
                delta -= 15
                note += " AI:suspicious"
            if ai_confidence in ("medium", "high") and ai_delta != 0:
                delta += ai_delta
                note += f" AI:{ai_delta:+d}"

            flags = scan.get("flags", [])
            meta = {
                "rating": rating,
                "mint_authority_revoked": scan.get("mint_authority_revoked"),
                "freeze_authority_revoked": scan.get("freeze_authority_revoked"),
                "bonding_fill_pct": scan.get("bonding_curve_fill_pct"),
                "dev_supply_pct": scan.get("dev_supply_pct"),
                "top_10_holder_pct": scan.get("top_10_holder_pct"),
                "deployer_wallet_age_days": scan.get("deployer_wallet_age_days"),
                "deployer_prior_launches": scan.get("deployer_prior_launches"),
                "scan_flags": (
                    ([f"🤖 {ai_warning}"] if ai_warning else []) +
                    [f["detail"] for f in flags if f.get("severity") in ("high", "critical") and f.get("detail")]
                ),
            }

            display_score = max(0, 15 + delta)
            return delta, CheckResult(score=display_score, max_score=30, reason=note), meta

        except Exception as e:
            return 0, CheckResult(score=15, max_score=30, reason=f"error: {e!s:.40}"), {}

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
        self._active.discard(mint)
