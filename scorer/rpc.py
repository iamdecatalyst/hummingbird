"""
Shared async Solana RPC client.
One httpx.AsyncClient reused across all checks — no per-request connection overhead.
"""
import httpx
from config import RPC_HTTP

_client: httpx.AsyncClient | None = None


def get_client() -> httpx.AsyncClient:
    global _client
    if _client is None or _client.is_closed:
        _client = httpx.AsyncClient(
            base_url=RPC_HTTP,
            timeout=5.0,
            limits=httpx.Limits(max_connections=20),
        )
    return _client


async def rpc(method: str, params: list) -> dict:
    """Fire a single JSON-RPC call, return the full response dict."""
    client = get_client()
    resp = await client.post(
        "/",
        json={"jsonrpc": "2.0", "id": 1, "method": method, "params": params},
    )
    resp.raise_for_status()
    return resp.json()


async def get_account_info(pubkey: str, encoding: str = "base64") -> dict | None:
    """Returns the value field of getAccountInfo, or None if account not found."""
    data = await rpc("getAccountInfo", [pubkey, {"encoding": encoding}])
    return data.get("result", {}).get("value")


async def get_signatures(pubkey: str, limit: int = 50) -> list[dict]:
    """Returns up to `limit` confirmed signatures for a given address."""
    data = await rpc(
        "getSignaturesForAddress",
        [pubkey, {"limit": limit, "commitment": "confirmed"}],
    )
    return data.get("result") or []
