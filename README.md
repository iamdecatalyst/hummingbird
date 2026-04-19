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
  <p>
    <a href="https://t.me/vylthummingbird"><img src="https://img.shields.io/badge/Telegram-Join_Community-24A1DE?style=for-the-badge&logo=telegram&logoColor=white" alt="Join Telegram"/></a>
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

## Required Services

Hummingbird needs three external services. All have free tiers.

<table>
<tr>
<td align="center" valign="top" width="120">
<img src="https://media.vylth.com/images/cbnHq6IQ.png" height="44" alt="Signet"/>
</td>
<td valign="top">
<h3>Signet — Wallet &amp; swap execution</h3>
<p>Non-custodial KMS — your keys live encrypted, signed on-demand. Powers every swap, transfer, and balance lookup.</p>
<a href="https://signet.vylth.com"><img src="https://img.shields.io/badge/Get_API_Key-→-22c55e?style=for-the-badge&labelColor=0a1510" alt="Signet"/></a>
<sub>&nbsp; Free: 1k req · 5 wallets · no card</sub>
</td>
</tr>
<tr><td colspan="2"><hr/></td></tr>
<tr>
<td align="center" valign="top" width="120">
<img src="https://media.vylth.com/images/zC4CjWmS.png" height="44" alt="Cricket"/>
</td>
<td valign="top">
<h3>Cricket — Risk score + smart-money signals</h3>
<p>Mantis pre-entry rug score. Firefly real-time accumulation / exodus signals from on-chain wallet flow — drives entry and exit.</p>
<a href="https://cricket.vylth.com"><img src="https://img.shields.io/badge/Get_API_Key-→-00A8FF?style=for-the-badge&labelColor=0a1015" alt="Cricket"/></a>
<sub>&nbsp; Free tier · no card</sub>
</td>
</tr>
<tr><td colspan="2"><hr/></td></tr>
<tr>
<td align="center" valign="top" width="120">
<img src="https://www.helius.dev/_next/image?url=%2Flogo.svg&w=128&q=75" height="44" alt="Helius"/>
</td>
<td valign="top">
<h3>Helius — Solana RPC</h3>
<p>WebSocket <code>logsSubscribe</code> + <code>getTransaction</code> with <code>jsonParsed</code>. Required for sub-100ms detection on pump.fun and friends.</p>
<a href="https://helius.dev"><img src="https://img.shields.io/badge/Get_API_Key-→-FFA500?style=for-the-badge&labelColor=1a1208" alt="Helius"/></a>
<sub>&nbsp; Free: 100k req/day · 10 RPS</sub>
</td>
</tr>
</table>

> **Why not public RPC?** Mainnet-beta is heavily rate-limited and drops WSS subscriptions under load. You'll detect maybe 1 in 10 launches. A private RPC is the difference between "interesting demo" and "actually trading."

---

## Self-Hosted Setup

**Prerequisites:** Rust, Python 3.11+, Go 1.22+, PostgreSQL

```bash
cp .env.example .env
# Required:
#   SIGNET_API_KEY, SIGNET_API_SECRET     — from signet.vylth.com
#   CRICKET_API_KEY                       — from cricket.vylth.com
#   RPC_HTTP, RPC_WS                      — Helius (or other private RPC)
#   DATABASE_URL                          — postgres://...
#   ENCRYPTION_KEY                        — `openssl rand -hex 32`
#   JWT_SECRET                            — `openssl rand -hex 32` (≥32 chars)
#   SCORER_SECRET                         — `openssl rand -hex 32` (≥32 chars, same in all 3 services)
```

```bash
# 1. Scorer
cd scorer && pip install -r requirements.txt && python main.py

# 2. Listener
cd listener && cargo run --release

# 3. Orchestrator
cd orchestrator && go run .
```

---

## Community

Join the Telegram channel for the live trade feed, support, and announcements:
**[t.me/vylthummingbird](https://t.me/vylthummingbird)**

---

## Creator

Built by **[@iamdecatalyst](https://github.com/iamdecatalyst)** — CEO & Solo Founder, [VYLTH Strategies](https://vylth.com).

---

## License

[Hummingbird License v1.0](./LICENSE) — free to use, self-host, and fork.
You may **not** rebrand it and sell it as a different product.
Violations will be pursued legally. Questions: decatalyst@vylth.com
