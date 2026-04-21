"""
Swing scanner — established token momentum plays.

Targets tokens that are 24h+ old with $50k+ liquidity.
Same DexScreener + Cricket confluence model as the scalper,
but with higher bars: stricter entry threshold, larger positions,
tighter stop-loss tolerance since moves are slower.

Discovery: DexScreener boosts/profiles (same feed as scalper, different age filter).
"""
import asyncio
import logging
import time
from typing import Optional

import httpx

from config import CRICKET_API_KEY, CRICKET_BASE_URL, SCORER_SECRET
from models import CheckResult, ScoreResult
from rpc import get_account_info

TOKEN_2022_PROGRAM = "TokenzQdBNbLqP5VEhdkAS6EPFLC1PHnBqCXEpPxuEb"

log = logging.getLogger(__name__)

DEXSCREENER_BASE   = "https://api.dexscreener.com"
SCAN_INTERVAL      = 60    # seconds — established tokens move slower, no need to poll every 30s
DISCOVER_INTERVAL  = 120   # seconds between discovery runs
BATCH_SIZE         = 30

MIN_AGE_MINUTES    = 1_440  # 24 hours
MAX_AGE_DAYS       = 30     # ignore zombie coins older than 30 days
MIN_LIQUIDITY      = 50_000 # $50k
DEXSCREENER_THRESHOLD = 45  # min DexScreener score before calling Cricket
ENTRY_THRESHOLD    = 65     # stricter than scalper (60)

_CRICKET_HEADERS = {"Authorization": f"Bearer {CRICKET_API_KEY}", "User-Agent": "hummingbird/1.0"}

_DEX_PLATFORM = {
    "pumpfun":   "pump_fun",
    "pumpswap":  "pump_fun",
    "launchlab": "raydium_launchlab",
    "moonshot":  "moonshot",
    "boop":      "boop",
}


class Swing:
    def __init__(self, orchestrator_url: str):
        self.orchestrator_url = orchestrator_url
        self._store: dict[str, dict] = {}   # mint → {platform, peak_price, added_ms}
        self._active: set[str] = set()
        self._broadcast: set[str] = set()
        self._last_discover = 0.0

    async def run(self):
        log.info("📈 Swing scanner started (interval: %ds, min_age: 24h, min_liq: $50k)", SCAN_INTERVAL)
        while True:
            await asyncio.sleep(SCAN_INTERVAL)
            try:
                now = time.time()
                if now - self._last_discover >= DISCOVER_INTERVAL:
                    self._last_discover = now
                    await self._discover()
                await self._scan()
            except Exception as e:
                log.error("[swing] loop error: %s", e)

    # ── Discovery ─────────────────────────────────────────────────────────────

    async def _discover(self):
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
                        if mint and mint not in self._store:
                            mints.add(mint)
                except Exception as e:
                    log.warning("[swing] discovery %s error: %s", endpoint, e)

            if not mints:
                return

            mint_list = list(mints)
            for i in range(0, len(mint_list), BATCH_SIZE):
                await self._store_batch(client, mint_list[i:i + BATCH_SIZE])

    async def _store_batch(self, client: httpx.AsyncClient, mints: list[str]):
        try:
            resp = await client.get(f"{DEXSCREENER_BASE}/tokens/v1/solana/{','.join(mints)}")
            if resp.status_code != 200:
                return
            pairs = resp.json()
            if not isinstance(pairs, list):
                return

            now_ms = int(time.time() * 1000)
            for pair in pairs:
                mint = (pair.get("baseToken") or {}).get("address")
                if not mint or mint in self._store:
                    continue
                liquidity = float((pair.get("liquidity") or {}).get("usd") or 0)
                if liquidity < MIN_LIQUIDITY:
                    continue
                created_at = pair.get("pairCreatedAt") or now_ms
                age_minutes = (now_ms - created_at) / 60_000
                if age_minutes < MIN_AGE_MINUTES or age_minutes > MAX_AGE_DAYS * 1440:
                    continue
                try:
                    acct = await get_account_info(mint)
                    if acct and acct.get("owner") == TOKEN_2022_PROGRAM:
                        log.info("[swing] ⛔ %s... Token 2022 — skipping", mint[:8])
                        continue
                except Exception:
                    pass

                dex_id = pair.get("dexId", "")
                platform = _DEX_PLATFORM.get(dex_id, dex_id or "unknown")
                price = float(pair.get("priceUsd") or 0)
                self._store[mint] = {"platform": platform, "peak_price": price, "added_ms": now_ms}
                age_h = int(age_minutes // 60)
                age_d = age_h // 24
                log.info("[swing] 🔎 discovered %s... dex=%s age=%dd%dh liq=$%.0f", mint[:8], dex_id, age_d, age_h % 24, liquidity)
        except Exception as e:
            log.warning("[swing] store_batch error: %s", e)

    # ── Scan ──────────────────────────────────────────────────────────────────

    async def _scan(self):
        eligible = [m for m in self._store if m not in self._active]
        if not eligible:
            return

        log.info("[swing] scanning %d eligible tokens", len(eligible))

        async with httpx.AsyncClient(timeout=10.0) as client:
            for i in range(0, len(eligible), BATCH_SIZE):
                batch = eligible[i:i + BATCH_SIZE]
                try:
                    resp = await client.get(f"{DEXSCREENER_BASE}/tokens/v1/solana/{','.join(batch)}")
                    if resp.status_code != 200:
                        continue
                    pairs = resp.json()
                    if not isinstance(pairs, list):
                        continue

                    pair_map: dict[str, dict] = {}
                    for pair in pairs:
                        addr = (pair.get("baseToken") or {}).get("address", "").lower()
                        if not addr:
                            continue
                        liq = float((pair.get("liquidity") or {}).get("usd") or 0)
                        if liq > float((pair_map.get(addr, {}).get("liquidity") or {}).get("usd") or 0):
                            pair_map[addr] = pair

                    candidates = []
                    for mint in batch:
                        pair = pair_map.get(mint.lower())
                        if not pair:
                            continue
                        result = self._score(mint, pair)
                        if result and result.total >= DEXSCREENER_THRESHOLD:
                            candidates.append((mint, result))
                except Exception as e:
                    log.error("[swing] scan batch error: %s", e)
                    continue

            if not candidates:
                return

            sem = asyncio.Semaphore(3)
            async with httpx.AsyncClient(timeout=6.0, headers=_CRICKET_HEADERS) as cc:
                tasks = [self._validate(cc, sem, mint, result) for mint, result in candidates]
                await asyncio.gather(*tasks, return_exceptions=True)

    # ── DexScreener scoring ───────────────────────────────────────────────────

    def _score(self, mint: str, pair: dict) -> Optional[ScoreResult]:
        now_ms = int(time.time() * 1000)
        price_usd = float(pair.get("priceUsd") or 0)
        if price_usd <= 0:
            return None

        stored = self._store.get(mint, {})
        peak = stored.get("peak_price", price_usd)
        if price_usd > peak:
            stored["peak_price"] = price_usd
            peak = price_usd

        price_change_5m = float((pair.get("priceChange") or {}).get("m5") or 0)
        price_change_1h = float((pair.get("priceChange") or {}).get("h1") or 0)
        price_change_24h = float((pair.get("priceChange") or {}).get("h24") or 0)
        vol_5m  = float((pair.get("volume") or {}).get("m5") or 0)
        vol_1h  = float((pair.get("volume") or {}).get("h1") or 0)
        vol_24h = float((pair.get("volume") or {}).get("h24") or 0)
        liquidity_usd = float((pair.get("liquidity") or {}).get("usd") or 0)
        txns_5m = (pair.get("txns") or {}).get("m5") or {}
        buys_5m = int(txns_5m.get("buys") or 0)
        sells_5m = int(txns_5m.get("sells") or 0)

        checks: dict[str, CheckResult] = {}
        total = 0

        # Pullback — 25pts (established tokens pull back and bounce more cleanly)
        pullback_pct = ((peak - price_usd) / peak * 100) if peak > 0 else 0
        if 15 <= pullback_pct <= 35:
            pts, reason = 25, f"ideal pullback ({pullback_pct:.1f}% from ATH)"
        elif 10 <= pullback_pct < 15:
            pts, reason = 15, f"shallow pullback ({pullback_pct:.1f}%)"
        elif 35 < pullback_pct <= 50:
            pts, reason = 10, f"deep pullback ({pullback_pct:.1f}%)"
        else:
            pts, reason = 0, f"no pullback ({pullback_pct:.1f}%)"
        checks["pullback"] = CheckResult(score=pts, max_score=25, reason=reason)
        total += pts

        # 5m momentum — 20pts
        if price_change_5m > 0 and vol_5m > 5_000:
            pts, reason = 20, f"momentum +{price_change_5m:.1f}% ${vol_5m:,.0f}"
        elif price_change_5m > 0:
            pts, reason = 10, f"price up, thin vol ${vol_5m:,.0f}"
        elif vol_5m > 5_000:
            pts, reason = 8, f"vol active, flat ${vol_5m:,.0f}"
        else:
            pts, reason = 0, "no momentum"
        checks["momentum"] = CheckResult(score=pts, max_score=20, reason=reason)
        total += pts

        # Buy pressure — 15pts
        total_txns = buys_5m + sells_5m
        if total_txns > 0:
            buy_ratio = buys_5m / total_txns
            if buy_ratio >= 0.65:
                pts, reason = 15, f"strong buyers {buys_5m}B/{sells_5m}S"
            elif buy_ratio >= 0.50:
                pts, reason = 8, f"neutral {buys_5m}B/{sells_5m}S"
            else:
                pts, reason = 0, f"sellers dominate {buys_5m}B/{sells_5m}S"
        else:
            pts, reason = 0, "no txns"
        checks["buy_pressure"] = CheckResult(score=pts, max_score=15, reason=reason)
        total += pts

        # 1h trend — 20pts (volume gated, same as scalper)
        if price_change_1h > 15 and vol_1h >= 100_000:
            pts, reason = 20, f"strong 1h +{price_change_1h:.0f}% ${vol_1h:,.0f}"
        elif price_change_1h > 15 and vol_1h >= 30_000:
            pts, reason = 14, f"1h trend +{price_change_1h:.0f}% ${vol_1h:,.0f}"
        elif price_change_1h > 5 and vol_1h >= 30_000:
            pts, reason = 10, f"positive 1h +{price_change_1h:.0f}% ${vol_1h:,.0f}"
        elif price_change_1h > 5:
            pts, reason = 4, f"positive 1h low vol ${vol_1h:,.0f}"
        elif price_change_1h > 0:
            pts, reason = 2, f"flat 1h +{price_change_1h:.0f}%"
        else:
            pts, reason = 0, f"dip 1h {price_change_1h:.0f}%"
        checks["trend_1h"] = CheckResult(score=pts, max_score=20, reason=reason)
        total += pts

        # 24h context — 10pts (established tokens: punish hard dumps, reward recovery)
        if price_change_24h > 20 and vol_24h >= 200_000:
            pts, reason = 10, f"strong 24h +{price_change_24h:.0f}% ${vol_24h:,.0f}"
        elif price_change_24h > 0:
            pts, reason = 5, f"positive 24h +{price_change_24h:.0f}%"
        elif price_change_24h > -20:
            pts, reason = 2, f"slight 24h {price_change_24h:.0f}%"
        else:
            pts, reason = 0, f"dumping 24h {price_change_24h:.0f}%"
        checks["trend_24h"] = CheckResult(score=pts, max_score=10, reason=reason)
        total += pts

        # Liquidity depth — 10pts (higher floor, clean exits matter more here)
        if liquidity_usd >= 500_000:
            pts, reason = 10, f"deep ${liquidity_usd:,.0f}"
        elif liquidity_usd >= 200_000:
            pts, reason = 8, f"${liquidity_usd:,.0f}"
        elif liquidity_usd >= 50_000:
            pts, reason = 5, f"${liquidity_usd:,.0f}"
        else:
            pts, reason = 0, f"low ${liquidity_usd:,.0f}"
        checks["liquidity"] = CheckResult(score=pts, max_score=10, reason=reason)
        total += pts

        return ScoreResult(
            mint=mint,
            platform=stored.get("platform", "unknown"),
            total=total,
            decision="swing",
            position_sol=0.0,
            checks=checks,
            scored_at_ms=now_ms,
        )

    # ── Cricket validation ────────────────────────────────────────────────────

    async def _validate(self, client: httpx.AsyncClient, sem: asyncio.Semaphore, mint: str, dex_result: ScoreResult):
        async with sem:
            cricket_delta, cricket_check, meta = await self._cricket_scan(client, mint)
            dex_result.checks["cricket"] = cricket_check

            if cricket_delta <= -100:
                log.info("[swing] ❌ %s... cricket veto: %s", mint[:8], cricket_check.reason)
                if mint not in self._broadcast:
                    self._broadcast.add(mint)
                    dex_result.decision = "skip"
                    dex_result.scan_flags = [f"🚫 {cricket_check.reason}"]
                    await self._forward(dex_result)
                return

            combined = max(0, min(100, dex_result.total + cricket_delta))
            dex_result.total = combined

            if combined < ENTRY_THRESHOLD:
                log.info("[swing] ⏭  %s... combined=%d — below threshold", mint[:8], combined)
                if mint not in self._broadcast:
                    self._broadcast.add(mint)
                    dex_result.decision = "skip"
                    skip_flags = []
                    if cricket_check.reason and cricket_check.reason not in ("unavailable", "no API key"):
                        skip_flags.append(f"Cricket: {cricket_check.reason}")
                    for name, chk in dex_result.checks.items():
                        if name != "cricket" and chk.score < chk.max_score // 2:
                            skip_flags.append(f"{name}: {chk.reason}")
                    dex_result.scan_flags = skip_flags
                    await self._forward(dex_result)
                return

            # Position sizing: established tokens get larger positions (cleaner exits)
            dex_result.position_sol = 0.20 if combined >= 80 else 0.10
            dex_result.decision = "swing"

            if meta:
                dex_result.rating = meta.get("rating", "")
                dex_result.mint_authority_revoked = meta.get("mint_authority_revoked")
                dex_result.freeze_authority_revoked = meta.get("freeze_authority_revoked")
                dex_result.dev_supply_pct = meta.get("dev_supply_pct")
                dex_result.top_10_holder_pct = meta.get("top_10_holder_pct")
                dex_result.deployer_wallet_age_days = meta.get("deployer_wallet_age_days")
                dex_result.deployer_prior_launches = meta.get("deployer_prior_launches")
                dex_result.scan_flags = meta.get("scan_flags", [])
                dex_result.ai_summary = meta.get("ai_summary", "")

            log.info(
                "[swing] 🎯 %s...  combined=%d/100  pos=%.2f SOL",
                mint[:8], combined, dex_result.position_sol,
            )
            for name, c in dex_result.checks.items():
                log.info("   %-14s %2d/%d  %s", name, c.score, c.max_score, c.reason)

            await self._forward(dex_result)

    async def _cricket_scan(self, client: httpx.AsyncClient, mint: str) -> tuple[int, CheckResult, dict]:
        if not CRICKET_API_KEY:
            return 0, CheckResult(score=0, max_score=30, reason="no API key"), {}
        try:
            resp = await client.get(f"{CRICKET_BASE_URL}/api/cricket/mantis/scan/{mint}")
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

            scan = data.get("scan", {})
            if scan.get("token_program") == "token-2022":
                return -100, CheckResult(score=0, max_score=30, reason="Token 2022 — exit unreliable"), {}

            if rating == "high":
                delta, note = -15, f"high risk ({base})"
            elif rating == "moderate":
                delta, note = 0, f"moderate ({base})"
            elif rating == "low":
                delta, note = +15, f"low risk ({base})"
            else:
                delta, note = 0, f"unknown ({base})"

            if not scan.get("mint_authority_revoked", True):
                delta -= 5; note += " mint_active"
            if not scan.get("freeze_authority_revoked", True):
                delta -= 5; note += " freeze_active"

            ai = data.get("ai_analysis") or {}
            ai_intent = ai.get("intent", "")
            ai_delta = int(ai.get("ai_risk_delta") or 0)
            ai_confidence = ai.get("confidence", "low")
            ai_warning = (ai.get("warning") or "").strip()

            if ai_intent == "likely_rug":
                return -100, CheckResult(score=0, max_score=30, reason="AI: likely rug — veto"), {}

            deployer_age = scan.get("deployer_wallet_age_days")
            lp_locked = scan.get("lp_locked", False)
            if ai_intent == "suspicious" and deployer_age is not None and deployer_age == 0 and not lp_locked:
                return -100, CheckResult(score=0, max_score=30, reason="suspicious AI + 0d deployer + no LP lock — veto"), {}

            if ai_intent == "suspicious":
                delta -= 15; note += " AI:suspicious"
            if ai_confidence in ("medium", "high") and ai_delta != 0:
                delta += ai_delta; note += f" AI:{ai_delta:+d}"

            top10 = scan.get("top_10_holder_pct")
            if top10 is not None:
                if top10 < 20:   delta += 10; note += f" top10={top10:.0f}%+10"
                elif top10 < 35: delta += 5;  note += f" top10={top10:.0f}%+5"
                elif top10 > 70: delta -= 15; note += f" top10={top10:.0f}%-15"
                elif top10 > 50: delta -= 8;  note += f" top10={top10:.0f}%-8"

            dev_pct = scan.get("dev_supply_pct")
            if dev_pct is not None and dev_pct > 15:
                delta -= 10; note += f" dev={dev_pct:.0f}%-10"

            flags = scan.get("flags", [])
            meta = {
                "rating": rating,
                "mint_authority_revoked": scan.get("mint_authority_revoked"),
                "freeze_authority_revoked": scan.get("freeze_authority_revoked"),
                "dev_supply_pct": scan.get("dev_supply_pct"),
                "top_10_holder_pct": scan.get("top_10_holder_pct"),
                "deployer_wallet_age_days": scan.get("deployer_wallet_age_days"),
                "deployer_prior_launches": scan.get("deployer_prior_launches"),
                "scan_flags": (
                    ([f"🤖 {ai_warning}"] if ai_warning else []) +
                    [f["detail"] for f in flags if f.get("severity") in ("high", "critical") and f.get("detail")]
                ),
                "ai_summary": (ai.get("summary") or "").strip(),
            }

            return delta, CheckResult(score=max(0, 15 + delta), max_score=30, reason=note), meta

        except Exception as e:
            return 0, CheckResult(score=15, max_score=30, reason=f"error: {e!s:.40}"), {}

    # ── Forward ───────────────────────────────────────────────────────────────

    async def _forward(self, signal: ScoreResult, write_log: bool = True):
        if write_log:
            try:
                from main import _write_score_log
                _write_score_log(signal, "swing")
            except Exception:
                pass
        url = f"{self.orchestrator_url}/trade"
        headers = {"Authorization": f"Bearer {SCORER_SECRET}"}
        try:
            async with httpx.AsyncClient(timeout=3.0) as client:
                resp = await client.post(url, json=signal.model_dump(), headers=headers)
                if resp.status_code == 200:
                    self._active.add(signal.mint)
                    log.info("[swing] → forwarded %s to orchestrator", signal.mint[:8])
                else:
                    log.error("[swing] orchestrator returned %d", resp.status_code)
        except Exception as e:
            log.error("[swing] forward error: %s", e)

    def on_position_closed(self, mint: str):
        self._active.discard(mint)
        self._broadcast.discard(mint)
