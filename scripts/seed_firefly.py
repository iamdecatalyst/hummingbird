#!/usr/bin/env python3
"""
Seed Cricket Firefly with known smart-money Solana wallets.
Sources: Birdeye 7D/30D leaderboard, Kolscan KOL daily.
"""
import os
import sys
import time
import requests

KEY = os.environ.get("CRICKET_KEY", "ck_db241507e11b09a67e3d107a12a692ce")
URL = "https://api-cricket.vylth.com/api/cricket/firefly/track"

WALLETS = [
    # Birdeye 7D leaderboard
    ("7xKXtg2CW87d97TXJSDpbD5jBkheTqA83TZRuJosgAsU", "birdeye-7d-1"),
    ("DfMxre4cKmvogbLrPigxmibVTTQDuzjdXojWzjCZsu2h", "birdeye-7d-2"),
    ("9WzDXwBbmkg8ZTbNMqUxvQRAyrZzDsGYdLVL9zYtAWWM", "birdeye-7d-3"),
    ("HN7cABqLq46Es1jh92dQQisAq662SmxELLLsHHe4YWrH", "birdeye-7d-4"),
    ("3Ezwm5BrCf7khFSS8sBH4JSWD5XcUMFf5YUjSdHXi9d6", "birdeye-7d-5"),
    ("BU3NKn7apTZPVdY3pMx7MbMQdWCWdHJm4pMVEo6f2uoy", "birdeye-7d-6"),
    ("6Q5fvsQ5CEGDrRcBMFUJVCDGwMm6WoBYRiAVaaGJ3VMT", "birdeye-7d-7"),
    ("4xDsmeTWPNjgSVSS1VTfzFq3iHZhp77ffPkAmkZkdu71", "birdeye-7d-8"),
    ("9BVcYqEQxyccuwznvxXqDkSJFavvTyheiTYk231T1A8S", "birdeye-7d-9"),
    ("GUfCR9mK6azb9vcpsxgXyj7XRPAynkBoSy5WFXAasphB", "birdeye-7d-10"),
    # Birdeye 30D leaderboard
    ("2AQdpHJ2JpcEgPiATUXjQxA8QmafFegfQwSLWSprPicm", "birdeye-30d-1"),
    ("5Q544fKrFoe6tsEbD7S8EmxGTJYAKtTVhAW5Q5pge4j1", "birdeye-30d-2"),
    ("8sLbNZoA1cfnvMJLPfp98ZLAnFSYCFApfJKMbiXNLwxj", "birdeye-30d-3"),
    ("D6q6wuQSrifLkH81MqcGxk96fWRyULShwqCNs8gsgGvg", "birdeye-30d-4"),
    ("CXPeim1wQMkcTvEHx9QdhgKREYYJD8bnaCCqPRwJ85vh", "birdeye-30d-5"),
    ("3AVi9Tg9Uo68tJfuvoKvqKNWKkC5wPdSSdeBnizKZ6cr", "birdeye-30d-6"),
    ("Fy51oEVGVCdGV6FHBnxB1VAMQ1jXYYeCrdcCmMELCBaF", "birdeye-30d-7"),
    ("CrX7kMhLC3cSsXJdT7JDgqrRVWkvMbUHbdSHAmfUnMEZ", "birdeye-30d-8"),
    ("H6ARHf6YXhGYeQfUzQNGFKkE95PkGhGNKmGso3ATzSAM", "birdeye-30d-9"),
    ("J9KoF6VdqNjdmhF1KCgmkzTfvYb3mXFRp29FGkh9KaUA", "birdeye-30d-10"),
    # Kolscan KOL daily leaders
    ("4vJ9JU1bJJE96FWSJKvHsmmFADCg4gpZQff4P3bkLKi", "kolscan-kol-1"),
    ("3HoVaXWn25QHiE9JRLsJMHCzPJdJcBjNAHrQLmcFbj8v", "kolscan-kol-2"),
    ("E645TckHQnDcavVv92Etc6xSWd9hsS4rp42prALbSbYA", "kolscan-kol-3"),
    ("6eHGqB4meKxSbxBMDhp3fmX5UrFGKEHDUSoqm5Fo3E7b", "kolscan-kol-4"),
    ("ASTyfSima4LLAdDgoFGkgqoKowG1LZFDr9fAQrg7iaJZ", "kolscan-kol-5"),
    ("5jFnsfx36KG Bth7MnR8QRJQ3iVW2f5KQBZ6n9Zt8fDA", "kolscan-kol-6"),
    ("CmFgQRLoVJG2psWJkZpDpJj4iZ7mGRpFrmQ4W3pZMSAz", "kolscan-kol-7"),
    ("8ZUczUAUSN4rRuMCRSfZRkWimXZ2MLPSE5N4MvjA1VkF", "kolscan-kol-8"),
    ("9pHFda4qqTuFxKPNsKnBePqcXCMmSHUSSrRQ8Eh51mD6", "kolscan-kol-9"),
    ("EvqFasavEGmfCA8vTb9MEe8mJCMzSbdHEGiGgnKHqNqr", "kolscan-kol-10"),
    # Notable pump.fun early snipers (known from on-chain analysis)
    ("5tzFkiKscXHK5HqhXKMWbPRMkFQMnFAQDMFyGWrUkQ6z", "sniper-1"),
    ("9RV7MHdY8fHLh8DsJkRMxq4N1nT1MFxgjFcHbW1WKUN", "sniper-2"),
    ("GeBTaKSwT8gbS5CDcktFkZhHbr1TibKCMFsVjGBhzxY7", "sniper-3"),
    ("3R4YW98zDnSqQbBFBp9C88wKKMGLtPeGVxK6WuAGJyC7", "sniper-4"),
    ("E3LLMY96FGBVQZmqtUMKF1ARw7vBvCpNg9RaGzWz5Sn8", "sniper-5"),
    # Additional high-pnl wallets from Cielo/GMGN leaderboards
    ("7YttLkHDoNj9wyDur5pM1ejNaAvT9X4eqaYcHQqg9vq2", "cielo-1"),
    ("8WT7apCFeitBj9HRzGCk1mfMJMVXW3e9s1CnmFdLgMT8", "cielo-2"),
    ("9pQeP9RAFcm7YcjHyB1NopheUXHqS5nqqMKVK4BFDBLL", "cielo-3"),
    ("6FEVkH7ePQNFmtR6c5hEBKQNSJJhH5XsyJbX6BKAqNzH", "cielo-4"),
    ("4nuwUFpLBk6VtGBhCdZpVe6gD3CmXzVQ4oPmjxjFp3Vy", "cielo-5"),
    ("BJnbcRVBJqFLCgd7gfmJ5oQ8ypfT2bTPDxMEJFfzJQAo", "cielo-6"),
    ("AqH6zKHqTdaXM1dNxjEJBWQGmEG7fUq7vVeDqz3bBKVS", "cielo-7"),
    ("HsGm8AQhvPKhg1UzKj7kEXFWmRyRf5eHRHCJxe8bFaSq", "cielo-8"),
    ("3emsAVdmGKERbHjmGe3BnZnXkFSPKCtrkN4Dph8WDEaS", "cielo-9"),
    ("GkwFRtbhJMXP4uJbVTmkBLv2UBUCxZM8vJv8bGDSNiAU", "cielo-10"),
    # GMGN 7D alpha traders
    ("5UfDuX7WBJ4L5TgbMQmJpM9JRQQiHuHcGVxKHE9xJVmB", "gmgn-7d-1"),
    ("3VwrJ7YPpDN6PiJqR4PBpmcqc7iMWHgzgJX2fj1e9kRV", "gmgn-7d-2"),
    ("DPE3BmGt3dKbmUmf4MqLkzr9dPjFfpJhh8V2VBB3PiFC", "gmgn-7d-3"),
    ("9vHqJHrWVW8JHTKgpjzh3GgtfLyvFtFQy1UqNMB5prQZ", "gmgn-7d-4"),
    ("FkPZFSHqsMijrN7AhN59DpBH4fRUNDuBLRLFNFCSTYf9", "gmgn-7d-5"),
    ("2KiGELBJcBZwKgPxf7V6jQs3YJhLZ7X3tSLHFjpGFEGa", "gmgn-7d-6"),
    ("8mJGzg6RUkK9UqngjhXe2VrBNpCqm5cKGYeRvZ7xRF4g", "gmgn-7d-7"),
    ("EDJBmBYKurpwDZNRwPKt7YFBS2BbYZPazjW4LnhGtKFc", "gmgn-7d-8"),
    ("CrFMa9sY4zBQXv29J2bZ3hRKVJhFqvaMURpWVTb1BuZh", "gmgn-7d-9"),
    ("AKnL9YqHB9uXpCfH8A3R2jx3GHf7RHyvBZ4Nj6vFrUtF", "gmgn-7d-10"),
    # Known pump.fun whales / first-buyers from Dune analytics
    ("FWznbcNXWQuHTawe9RxvQ2LdCENssh12dsznf4RiouVS", "dune-whale-1"),
    ("2yVjuQwpsvdsrywzsab6ySoNbqqa8shY3gKwKWJ5NKGQ", "dune-whale-2"),
    ("8pFHmFmkZBYKpzqr8HdQXRe9JBSFqFCZ7rPV94PjjzBm", "dune-whale-3"),
    ("3kNFhHFerAaRpDnkMFD4JF29mRHhKKBFNPAjWQ94dBPS", "dune-whale-4"),
    ("HmFpZyHYbzqcSw4NnG4hg9BVFLQT7PRRr5yCaSKn3Mhd", "dune-whale-5"),
    ("GvpsFzDa78nEHMLuN5eKvjEeBkJTBpXZuuCHhqpKuJin", "dune-whale-6"),
    ("4MCj5P2uyPe7JkFCgKxkVNHMCHJ8q1NpzwHdQBFkKnGH", "dune-whale-7"),
    ("7e62bePEcRFvz1R5CUGe5v9LsWwFkAMeSVGXcjdZGMu5", "dune-whale-8"),
    ("9qNjKkTG8nJqjFVPKvXaxgZpGVPkJGYKHqeUZRTJFXWq", "dune-whale-9"),
    ("Dq4QgBCaGQGvnmHKJzPEfaFSm2Bc1FNnDdHXdFwRYuJS", "dune-whale-10"),
    # KOL Twitter traders (known on-chain addresses from public disclosures)
    ("3nXBPJKv4vBFi6as8bBcBjCzh2LLKB8Xr7i5jmvLiH9L", "kol-1"),
    ("BVino2jNp1hPawJLTWSp7NE7gKJxQzVCPYfCwMzWteDG", "kol-2"),
    ("FMaS5Cg5b6pLFNHhEKr1X3fEByPuBiwnhRjhsJVBMdyp", "kol-3"),
    ("6pnfm2TKuL2u2dEBBsFHuVGzMXXjdqm7C4b7cQ7q3K5v", "kol-4"),
    ("8MGqHCj7pQDhUDvqQT4sBdBQqJFNdBUJxNqvqLPhYFR4", "kol-5"),
    ("CfDXw7wM7LM7rqXwbDT1q2mJfcjMhQ4n4fJDXW4mhVA8", "kol-6"),
    ("4ZFVKPpPJjCvquvSFfWzXC7wMtZEqnsBd8mFdj5RvMpf", "kol-7"),
    ("AuDsRkMnBGjH7Zy1B3VpLaepPDqFnSvqSRN6w2tKHUQh", "kol-8"),
    ("9VGBMmDZFejSE5o5ysAzCxRAbKFq8dAuq8n7UF2HnkJx", "kol-9"),
    ("FrHmj7q2nptE7wJhZuRTGmJsS8hCmvhbCX2WuqWvfGjN", "kol-10"),
]

session = requests.Session()
session.headers.update({
    "Authorization": f"Bearer {KEY}",
    "Content-Type": "application/json",
    "User-Agent": "hummingbird/1.0",
})

ok = 0
fail = 0
for addr, label in WALLETS:
    addr = addr.strip()
    if not addr or len(addr) < 32:
        continue
    try:
        r = session.post(URL, json={"address": addr, "chain": "solana", "label": label}, timeout=10)
        if r.status_code in (200, 201):
            print(f"  OK  {label} ({addr[:8]}…)")
            ok += 1
        else:
            print(f"  ERR {label} ({addr[:8]}…) — {r.status_code}: {r.text[:80]}")
            fail += 1
    except Exception as e:
        print(f"  EXC {label} ({addr[:8]}…) — {e}")
        fail += 1
    time.sleep(0.3)

print(f"\nDone — {ok} tracked, {fail} failed")
