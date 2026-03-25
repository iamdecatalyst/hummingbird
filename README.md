```
  _                     _                 _     _         _
 | |__  _   _ _ __ ___ | |_ ___ __  __ (_) __| |       | |
 | '_ \| | | | '_ ` _ \| __/ __|  \/  || |/ _` |       | |
 | | | | |_| | | | | | | || (__ | |\/| || | (_| |  _   | |
 |_| |_|\__,_|_| |_| |_|\__\___|_|  |_||_|\__,_| (_)  |_|

         autonomous pump.fun trading agent
         rust · python · go · solana
```

> Detects new token launches in **<100ms**, scores rug risk before entering,
> scalps momentum on already-running tokens, and exits before the dev dumps.
> All non-custodial — powered by [Signet](https://signet.vylth.com).

---

## How It Works

```
 ┌─────────────────────────────────────────────────────────────────┐
 │                        TWO MODES                                │
 │                                                                 │
 │   SNIPER                          SCALPER                       │
 │   ──────                          ───────                       │
 │   New token launches              Already-running tokens        │
 │   Enter early, ride the pump      Spot second-wave momentum     │
 │   Exit before the rug             Tight in/out, fast profits    │
 └─────────────────────────────────────────────────────────────────┘

 ┌──────────────────────────────────────────────────────────────────────┐
 │                                                                      │
 │   pump.fun WebSocket  ──►  new mint detected  (Rust, <100ms)        │
 │                                    │                                 │
 │                                    ▼                                 │
 │                         score token (Python)                         │
 │                         · dev wallet age & history                   │
 │                         · dev % of supply held                       │
 │                         · contract flags (mint/freeze auth)          │
 │                         · LP status                                  │
 │                         · bonding curve fill %                       │
 │                         · social presence                            │
 │                                    │                                 │
 │              ┌─────────────────────┴──────────────────────┐         │
 │              │ score < 60 → skip    score ≥ 60 → ENTER    │         │
 │              └─────────────────────────────────────────────┘         │
 │                                    │                                 │
 │                                    ▼                                 │
 │                    execute via Signet API  (Go)                      │
 │                    non-custodial · your keys                         │
 │                                    │                                 │
 │                                    ▼                                 │
 │                      watch position  (Rust)                          │
 │                      · dev wallet selling?  → DUMP NOW              │
 │                      · LP removed?          → DUMP NOW              │
 │                      · 2x hit?              → sell 40%              │
 │                      · 5x hit?              → sell 40%              │
 │                      · -25%?                → stop loss             │
 │                                    │                                 │
 │                                    ▼                                 │
 │              🐦 Telegram: "Exited TICKER | +340% | 0.43 SOL"        │
 │                                    │                                 │
 │                                    ▼                                 │
 │                         profits recycled → next cycle               │
 │                                                                      │
 └──────────────────────────────────────────────────────────────────────┘
```

---

## Scoring

```
 PRE-ENTRY SCORE  (0 – 100)
 ──────────────────────────────────────────────────────
 dev wallet age & rug history    0 – 20 pts
 dev % of supply held            0 – 20 pts   (>20% = red flag)
 contract flags                  0 – 15 pts   (mint/freeze auth)
 LP status                       0 – 20 pts
 social presence                 0 – 10 pts
 bonding curve fill              0 – 15 pts   (sweet spot: 5–25%)
 ──────────────────────────────────────────────────────
 < 60   →  skip
 60–74  →  0.05 SOL
 75–89  →  0.10 SOL
 90+    →  0.20 SOL
```

---

## Exit Logic

```
 EMERGENCY (no hesitation)
   dev sells >5% of holdings      →  dump everything now
   LP removed                     →  dump everything now
   >15% drop in under 10 seconds  →  dump everything now

 TAKE PROFIT (staged)
   2x entry price   →  sell 40%
   5x entry price   →  sell 40%
   10x entry price  →  sell remaining  (or trail)

 STOP LOSS
   -25% from entry               →  cut and move on
   -15% AND volume dying         →  don't wait for -25%

 TIME EXIT
   no movement after 8 minutes   →  exit at market
   bonding curve stalled <10%    →  exit after 5 min
```

---

## Scalper Mode

```
 while True:
   scan tokens launched in last 30 minutes
   find second-wave pattern:
     · pumped → pulled back 20–40% from ATH
     · volume rising again
     · holder count still growing
     · dev wallet untouched
   enter tight position (0.05–0.10 SOL)
   target 1.5–2x, stop at -15%
   take and go, repeat
```

---

## Modules

```
 hummingbird/
 ├── listener/        Rust  — Solana WebSocket detector + position monitor
 ├── scorer/          Python — token risk scoring engine
 ├── orchestrator/    Go    — Signet integration, portfolio manager
 └── bot/             Go    — Telegram + Discord interface
```

---

## Quick Start

```bash
cp .env.example .env
# Set your RPC endpoint and Signet API key

# 1. scorer
cd scorer && pip install -r requirements.txt && python main.py

# 2. listener
cd listener && cargo run --release

# 3. orchestrator + bot
cd orchestrator && go run .
```

You need a **Signet API key** to execute trades → [signet.vylth.com](https://signet.vylth.com)
Free tier: 1,000 requests · 5 wallets · no credit card

---

## RPC

For real detection speed, use a private RPC (public mainnet rate-limits under load):

- [Helius](https://helius.xyz) — recommended
- [QuickNode](https://quicknode.com)
- [Triton](https://triton.one)

---

## License

MIT — free to use, fork, and build on.
