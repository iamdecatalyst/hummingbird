<div align="center">
  <img src="https://media.vylth.com/images/Inhr2N9T.png" width="110" alt="Hummingbird" />
  <h1>Hummingbird</h1>
  <p><strong>Autonomous pump.fun trading agent</strong></p>
  <p>
    <img src="https://img.shields.io/badge/Rust-listener-orange?style=flat-square&logo=rust" />
    <img src="https://img.shields.io/badge/Python-scorer-blue?style=flat-square&logo=python" />
    <img src="https://img.shields.io/badge/Go-orchestrator-00ADD8?style=flat-square&logo=go" />
    <img src="https://img.shields.io/badge/Solana-non--custodial-9945FF?style=flat-square&logo=solana" />
  </p>
</div>

---

Detects new token launches in **<100ms**, scores rug risk before entering, scalps momentum on already-running tokens, and exits before the dev dumps. All non-custodial — powered by [Signet](https://signet.vylth.com).

---

## How It Works

```
                    TWO MODES
        ┌───────────────────────────────────┐
        │  SNIPER            SCALPER        │
        │  ──────            ───────        │
        │  New launches      Running tokens │
        │  Enter early       Second wave    │
        │  Ride the pump     Tight in/out   │
        └───────────────────────────────────┘

  pump.fun WebSocket
        │
        ▼  <100ms (Rust)
  new mint detected
        │
        ▼
  score token (Python)
  ├── dev wallet age & history   (0–20 pts)
  ├── dev % of supply held       (0–20 pts)
  ├── bonding curve fill %       (0–20 pts)
  ├── contract flags             (0–15 pts)
  ├── social presence            (0–10 pts)
  └── LP status                  (0–15 pts)
        │
        ├── score < 60 → skip
        └── score ≥ 60 → ENTER
                │
                ▼
        Signet API swap (Go)
        non-custodial · your keys
                │
                ▼
        watch position
        ├── dev selling?          → dump now
        ├── LP removed?           → dump now
        ├── -15% in <10s?         → dump now
        ├── 2x hit?               → sell 40%
        ├── 5x hit?               → sell 40%
        ├── 10x hit?              → sell rest
        ├── -25% from entry?      → stop loss
        └── 8 min no movement?    → time exit
                │
                ▼
        Telegram: "Exited TICKER | +340% | 0.43 SOL"
```

---

## Scoring

| Check | Points | Signal |
|---|---|---|
| Dev wallet age & rug history | 0–20 | Fresh wallet = high risk |
| Dev % of supply held | 0–20 | >20% held = likely dump incoming |
| Bonding curve fill | 0–20 | Sweet spot: 5–25% filled |
| Contract flags | 0–15 | Mint/freeze authority = red flag |
| Social presence | 0–10 | No socials = anonymous dev |
| LP status | 0–15 | Locked LP = safer entry |

```
< 60    →  skip
60–74   →  enter 0.05 SOL
75–89   →  enter 0.10 SOL
90+     →  enter 0.20 SOL
```

---

## Scalper Mode

Runs continuously alongside the sniper. Targets tokens already launched in the last 30 minutes showing a second-wave momentum pattern:

- Pumped → pulled back 20–40% from ATH
- Volume rising again
- Holder count still growing
- Dev wallet untouched

Enters tight (0.05–0.10 SOL), targets 1.5–2x, stops at -15%, repeats.

---

## Project Structure

```
hummingbird/
├── listener/       Rust   — Solana WebSocket detector, <100ms detection
├── scorer/         Python — concurrent risk scoring engine
│   └── checks/            dev_wallet · supply · bonding · contract · social
├── orchestrator/   Go     — Signet integration, portfolio, Telegram bot
│   ├── trader/            execution + exit loop
│   ├── monitor/           per-position price watcher
│   ├── portfolio/         open/closed position state
│   ├── userbot/           multi-tenant user management
│   ├── bot/               Telegram bot (single + multi-tenant)
│   └── db/                PostgreSQL persistence
└── cli/            Go     — terminal UI for local/self-hosted installs
```

---

## CLI — Quick Install

No Go required. Works on Linux, macOS, Windows:

```bash
npm install -g @decatalyst/hummingbird
hummingbird          # interactive TUI dashboard
hummingbird login    # save your API credentials
hummingbird status   # one-shot stats
```

---

## Self-Hosted Setup

**Prerequisites:** Rust, Python 3.11+, Go 1.22+, PostgreSQL

```bash
cp .env.example .env
# Fill in: RPC_HTTP, SIGNET_API_KEY, SIGNET_API_SECRET, DATABASE_URL
```

```bash
# 1. Scorer
cd scorer && pip install -r requirements.txt && python main.py

# 2. Listener
cd listener && cargo run --release

# 3. Orchestrator
cd orchestrator && go run .
```

You need a **Signet API key** to execute trades → [signet.vylth.com](https://signet.vylth.com)
Free tier: 1,000 requests · 5 wallets · no credit card.

---

## RPC

For real detection speed, use a private RPC — public mainnet rate-limits under load:

- [Helius](https://helius.xyz) — recommended
- [QuickNode](https://quicknode.com)
- [Triton](https://triton.one)

---

## Creator

Built by **[@iamdecatalyst](https://github.com/iamdecatalyst)** — CEO & Solo Founder, [VYLTH Strategies](https://vylth.com).

---

## License

[Hummingbird License v1.0](./LICENSE) — free to use, self-host, and fork.
You may **not** rebrand it and sell it as a different product.
Violations will be pursued legally. Questions: decatalyst@vylth.com
