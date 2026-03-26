"""
Token store — keeps a rolling window of detected tokens for the scalper to scan.

Every token the Rust listener detects flows through here.
The scalper reads from here every 30 seconds looking for second-wave patterns.
"""
import time
from dataclasses import dataclass, field
from typing import Optional


@dataclass
class StoredToken:
    mint: str
    platform: str
    chain: str
    dev_wallet: str
    bonding_curve: str
    first_seen_ms: int

    # Updated by scalper as it tracks price
    peak_price_usd: float = 0.0
    last_price_usd: float = 0.0
    last_checked_ms: int = 0


class TokenStore:
    """
    In-memory store for recently detected tokens.
    Thread-safe for asyncio (single-threaded event loop).
    """

    def __init__(self, max_age_minutes: int = 45):
        self._tokens: dict[str, StoredToken] = {}
        self._max_age_ms = max_age_minutes * 60 * 1000

    def add(self, mint: str, platform: str, chain: str, dev_wallet: str, bonding_curve: str, timestamp_ms: int):
        """Add a newly detected token. Ignored if already stored."""
        if mint not in self._tokens:
            self._tokens[mint] = StoredToken(
                mint=mint,
                platform=platform,
                chain=chain,
                dev_wallet=dev_wallet,
                bonding_curve=bonding_curve,
                first_seen_ms=timestamp_ms,
            )

    def get_eligible(self, min_age_minutes: float = 5.0) -> list[StoredToken]:
        """
        Return tokens that are old enough to have formed a pump+pullback
        but not too old for a second wave to matter.
        """
        now_ms = int(time.time() * 1000)
        min_age_ms = int(min_age_minutes * 60 * 1000)

        return [
            t for t in self._tokens.values()
            if min_age_ms <= (now_ms - t.first_seen_ms) <= self._max_age_ms
        ]

    def update_price(self, mint: str, price_usd: float):
        """Track peak price for pullback depth calculation."""
        if mint not in self._tokens:
            return
        t = self._tokens[mint]
        if price_usd > t.peak_price_usd:
            t.peak_price_usd = price_usd
        t.last_price_usd = price_usd
        t.last_checked_ms = int(time.time() * 1000)

    def cleanup(self):
        """Evict tokens older than max_age. Call periodically."""
        now_ms = int(time.time() * 1000)
        stale = [
            mint for mint, t in self._tokens.items()
            if now_ms - t.first_seen_ms > self._max_age_ms
        ]
        for mint in stale:
            del self._tokens[mint]

    def size(self) -> int:
        return len(self._tokens)

    def get(self, mint: str) -> Optional[StoredToken]:
        return self._tokens.get(mint)
