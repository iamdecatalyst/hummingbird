"""
Social presence check — 0 to 10 points.

Fetches the Metaplex token metadata, reads the off-chain JSON URI,
and checks for Twitter and Telegram links.

A token with no socials is anonymous by design — high rug risk.
One with both Twitter and Telegram has at minimum a paper trail.
"""
import hashlib
import struct
import base64
import httpx
from rpc import get_account_info
from config import TOKEN_METADATA_PROGRAM
from models import CheckResult

MAX_SCORE = 10

# Seeds for Metaplex metadata PDA
_METADATA_SEED = b"metadata"


async def check(mint: str) -> CheckResult:
    try:
        metadata_pda = _derive_metadata_pda(mint)
        account = await get_account_info(metadata_pda, encoding="base64")
        uri = _parse_uri(account)
        if not uri:
            return CheckResult(score=0, max_score=MAX_SCORE, reason="no metadata URI")
        return await _check_uri(uri)
    except Exception as e:
        return CheckResult(score=0, max_score=MAX_SCORE, reason=f"error: {e}")


def _derive_metadata_pda(mint: str) -> str:
    """
    Derives the Metaplex metadata PDA for a given mint.
    Seeds: ["metadata", TOKEN_METADATA_PROGRAM_PUBKEY, mint_pubkey]
    """
    import base58  # type: ignore

    program_id = base58.b58decode(TOKEN_METADATA_PROGRAM)
    mint_bytes = base58.b58decode(mint)
    seeds = [_METADATA_SEED, program_id, mint_bytes]

    # find_program_address: iterate nonce 255..0 until valid
    for nonce in range(255, -1, -1):
        try:
            pda = _create_program_address(seeds + [bytes([nonce])], program_id)
            return base58.b58encode(pda).decode()
        except Exception:
            continue

    raise ValueError(f"Could not derive metadata PDA for {mint}")


def _create_program_address(seeds: list[bytes], program_id: bytes) -> bytes:
    """
    Solana create_program_address — SHA256 hash of seeds + program_id + "ProgramDerivedAddress".
    Raises if the result is on the Ed25519 curve (invalid PDA).
    """
    import hashlib
    h = hashlib.sha256()
    for seed in seeds:
        h.update(seed)
    h.update(program_id)
    h.update(b"ProgramDerivedAddress")
    result = h.digest()

    # Check it's off-curve (valid PDA must not be a valid Ed25519 point)
    if _is_on_curve(result):
        raise ValueError("PDA is on curve")
    return result


def _is_on_curve(point: bytes) -> bool:
    """Very simplified curve check — good enough for PDA derivation."""
    # In practice, ~50% of random 32-byte values are valid Ed25519 points.
    # We rely on the nonce loop above to find one that's off-curve.
    try:
        from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PublicKey
        Ed25519PublicKey.from_public_bytes(point)
        return True
    except Exception:
        return False


def _parse_uri(account: dict | None) -> str | None:
    """
    Parse the metadata URI out of the Metaplex metadata account.
    The URI is a 4-byte length-prefixed UTF-8 string starting at offset 69
    (after discriminator + name field).
    """
    if not account:
        return None
    data_b64 = account.get("data")
    if not data_b64 or not isinstance(data_b64, list):
        return None
    try:
        data = base64.b64decode(data_b64[0])
        # Metaplex metadata layout (simplified):
        # [0]      key (1 byte)
        # [1-32]   update_authority
        # [33-64]  mint
        # [65-68]  name length (u32 LE)
        # [69+]    name (up to 32 bytes padded)
        # then symbol, then URI
        if len(data) < 70:
            return None
        offset = 1 + 32 + 32  # key + update_authority + mint = 65
        name_len = struct.unpack_from("<I", data, offset)[0]
        offset += 4 + min(name_len, 32)  # skip name
        if offset + 4 > len(data):
            return None
        symbol_len = struct.unpack_from("<I", data, offset)[0]
        offset += 4 + min(symbol_len, 10)  # skip symbol
        if offset + 4 > len(data):
            return None
        uri_len = struct.unpack_from("<I", data, offset)[0]
        offset += 4
        if offset + uri_len > len(data):
            return None
        uri = data[offset:offset + uri_len].decode("utf-8", errors="ignore").rstrip("\x00")
        return uri if uri.startswith("http") else None
    except Exception:
        return None


async def _check_uri(uri: str) -> CheckResult:
    """Fetch the off-chain JSON and check for Twitter + Telegram links."""
    try:
        async with httpx.AsyncClient(timeout=3.0) as client:
            resp = await client.get(uri)
            resp.raise_for_status()
            meta = resp.json()
    except Exception:
        return CheckResult(score=2, max_score=MAX_SCORE, reason="metadata URI unreachable")

    score = 0
    found = []

    # Check top-level fields and nested extensions
    text = str(meta).lower()
    if "twitter.com" in text or "x.com" in text:
        score += 5
        found.append("Twitter")
    if "t.me/" in text or "telegram" in text:
        score += 5
        found.append("Telegram")

    if not found:
        reason = "no Twitter or Telegram found"
    else:
        reason = f"found: {', '.join(found)}"

    return CheckResult(score=score, max_score=MAX_SCORE, reason=reason)
