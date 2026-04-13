# Hummingbird — Agent Instructions

Autonomous pump.fun trading agent. Rust + Python + Go.
**Repo:** https://github.com/iamdecatalyst/hummingbird (public)
**Strategic purpose:** Open source viral project → drives Signet API signups → VYLTH revenue.

---

## Server

**Host:** Sage — `decatalyst@89.117.52.141`
**Code path:** `/opt/hummingbird`
**Deploy:** `cd /opt/hummingbird && git pull` then rebuild per service below

### Services

| Service | Binary | Restart |
|---------|--------|---------|
| `hummingbird-listener.service` | `/opt/hummingbird/bin/hummingbird-listener` | `sudo systemctl restart hummingbird-listener` |
| `hummingbird-orchestrator.service` | `/opt/hummingbird/bin/orchestrator` | `sudo systemctl restart hummingbird-orchestrator` |
| `hummingbird-scorer.service` | Python FastAPI at `localhost:8001` | `sudo systemctl restart hummingbird-scorer` |

### Build commands (on server)

```bash
# Orchestrator (Go)
cd /opt/hummingbird/orchestrator && /usr/local/go/bin/go build -o ../bin/hummingbird-orchestrator .
cp /opt/hummingbird/bin/hummingbird-orchestrator /opt/hummingbird/bin/orchestrator
sudo systemctl restart hummingbird-orchestrator

# Listener (Rust) — cross-compile from dev machine, rsync binary
# On dev: cargo build --release --target x86_64-unknown-linux-gnu
# rsync target/x86_64-unknown-linux-gnu/release/hummingbird-listener decatalyst@89.117.52.141:/opt/hummingbird/bin/
sudo systemctl restart hummingbird-listener

# Web (React + Vite)
cd /opt/hummingbird/web && npm run build
# Nginx serves /opt/hummingbird/web/dist at hummingbird.vylth.com

# Scorer (Python) — no build needed, systemd runs uvicorn directly
sudo systemctl restart hummingbird-scorer
```

### Environment

Config at `/opt/hummingbird/.env` — loaded by all services.

Key vars:
```
MULTI_TENANT=true
DATABASE_URL=postgres://...
ENCRYPTION_KEY=<64 hex chars>
JWT_SECRET=<secret>
TELEGRAM_TOKEN=<bot token>
RPC_HTTP=https://divine-frequent-spree.solana-mainnet.quiknode.pro/<key>
RPC_WS=wss://divine-frequent-spree.solana-mainnet.quiknode.pro/<key>
SOLANA_RPC=https://divine-frequent-spree.solana-mainnet.quiknode.pro/<key>
SIGNET_BASE_URL=https://api.signet.vylth.com/v1
```

**RPC note:** QuickNode free tier has a **daily HTTP request limit** — if scorer/listener stall with null results, the limit is exhausted. Wait for midnight UTC or upgrade to NodeReal.

---

## Architecture

```
listener/       Rust — Solana WebSocket listener + EVM listener
                Detects new token launches on pump.fun, raydium_launchlab, boop, moonshot + EVM
                POSTs TokenDetected to scorer at localhost:8001/score

scorer/         Python (FastAPI + asyncio) — pre-entry rug risk scoring
                Runs 5 checks concurrently: dev_wallet, supply, bonding, contract, social
                Returns ScoreResult with decision (skip/small/medium/full) + position_sol
                POSTs to orchestrator at localhost:8002/trade

orchestrator/   Go — multi-tenant trading engine
                Receives ScoreResult → fans out to all active user instances
                Each instance: Portfolio + Trader + Monitor (per-position)
                Signet SDK for wallet operations (swap SOL→token, token→SOL)
                Telegram bot for alerts + interactive config
                Web API at :8002

web/            React 19 + Vite + TypeScript + Tailwind
                Dashboard at hummingbird.vylth.com
                Nexus SSO auth → JWT → per-user data
```

---

## Orchestrator package layout

```
orchestrator/
├── main.go             HTTP server — all API endpoints, multi-tenant startup
├── config/config.go    Global env config (port, RPC URL, JWT secret, etc.)
├── db/db.go            Postgres — hb_users + hb_user_configs tables, AES-256-GCM encryption
├── auth/auth.go        JWT issue/parse
├── bot/
│   ├── bot.go          Telegram bot — multi-tenant, inline keyboard, per-user config callbacks
│   └── render.go       All message templates + BotConfig struct
├── userbot/manager.go  Per-user Portfolio + Trader instances
├── portfolio/          Position tracking, P&L, daily loss limit
├── trader/trader.go    Signet swap execution + exit handler
├── monitor/monitor.go  Per-position price watcher — SL/TP/timeout → ExitSignal
├── models/models.go    Shared types (Position, ClosedPosition, ScoreResult, etc.)
├── alerts/telegram.go  Telegram push notifications (entered/exited/alert)
└── eventlog/           In-memory event log for /logs endpoint
```

---

## API endpoints (orchestrator :8002)

```
GET  /mode                  → { multi_tenant: bool }
GET  /health

POST /auth/nexus            → exchange Nexus access_token → JWT
GET  /auth/me               → profile + bot_active
POST /auth/setup-signet     → first-time Signet key setup → starts bot
DELETE /auth/signet         → remove credentials + stop bot
POST /auth/telegram/token   → generate deep-link token for Telegram
POST /auth/cli-token        → 7-day token for CLI

GET  /stats                 → portfolio stats + wallet balance
GET  /positions             → open positions
GET  /closed                → last 50 closed trades
GET  /logs                  → event log
GET  /config                → per-user UserConfig from DB
PUT  /config                → save UserConfig + restart bot instance
POST /stop                  → stop user's bot instance
POST /resume                → resume (or restart) user's bot

GET  /wallets               → list Signet wallets with SOL balance
POST /wallets               → create wallet
POST /wallets/{id}/set-main → set trading wallet
POST /wallets/{id}/withdraw → transfer SOL

POST /trade                 → internal (scorer → orchestrator) score result fan-out
```

---

## Per-user config system (added 2026-03-27)

Each user has a row in `hb_user_configs` (JSONB). Defaults:

```go
UserConfig{
    SniperEnabled:   true,
    ScalperEnabled:  true,
    MaxPositionSOL:  0.10,
    MaxPositions:    5,
    StopLossPercent: 0.25,   // 25% stop loss per trade
    DailyLossLimit:  0.30,   // pause portfolio at -30%
    TakeProfit1x:    2.0,    // 2x → sell 40%
    TakeProfit2x:    5.0,    // 5x → sell 40%
    TakeProfit3x:    10.0,   // 10x → sell rest
    TimeoutMinutes:  8,
    MinBalanceSOL:   0.0,
}
```

**Changing config** (via web PUT /config or Telegram bot buttons) immediately stops and restarts the user's bot instance with the new settings. TP/SL/timeout flow through to `monitor.MonitorConfig`.

---

## Scorer checks

```
scorer/checks/
  dev_wallet.py   20pts — wallet age, tx history, SOL balance
  supply.py       20pts — dev % of total token supply
  bonding.py      20pts — pump.fun bonding curve fill % (5-25% sweet spot)
                         Other platforms return neutral 10/20
  contract.py     15pts — mint/freeze authority flags
  social.py       10pts — Twitter + Telegram presence
```

Score thresholds (scorer/config.py):
- `SKIP_BELOW` → skip
- `SMALL_BELOW` → small position
- `MEDIUM_BELOW` → medium position
- else → full position

---

## Listener platforms

```
listener/src/
  listener.rs     WebSocket manager — Solana + EVM, reconnect loops
  fetcher.rs      getTransaction (jsonParsed encoding) — platform-aware account parsing
  parser.rs       Log parsing — detects new token launch instructions
  forwarder.rs    HTTP POST to scorer

Solana platforms:
  pump_fun          accounts[0]=dev, [1]=mint, [3]=bonding_curve
  raydium_launchlab accounts[0]=dev, [2]=pool_state, [4]=mint
  boop              accounts[0]=dev, [1]=mint, [2]=bonding_curve
  moonshot          (configured similarly)

EVM platforms:
  Topics[1] = token address (last 20 bytes of 32-byte padded topic)
  Topics[2] = creator address
```

**Critical:** Must use `jsonParsed` encoding for `getTransaction` — `json` encoding returns null for V0 (versioned) transactions used by raydium_launchlab.

---

## Database

```sql
hb_users (nexus_user_id PK, username, signet_key BYTEA encrypted, wallet_id, main_wallet_id, telegram_chat_id, ...)
hb_user_configs (nexus_user_id PK FK, config_json JSONB)
```

AES-256-GCM encryption for Signet credentials. Key = ENCRYPTION_KEY env var (64 hex chars).

---

## Telegram bot

- **Bot:** @dehummingbirdbot
- Multi-tenant: one bot, resolves user by chat_id → nexus_user_id mapping
- Deep-link linking: user generates token from dashboard → clicks `t.me/dehummingbirdbot?start=<token>`
- Commands: `/start`, `/menu`, `/stats`, `/positions`, `/config`, `/stop`, `/pause`, `/resume`
- Config has inline +/- buttons for: position size, max positions, stop loss, TP1/2/3, timeout

---

## Web dashboard

```
web/src/
  pages/Dashboard.tsx     Main dashboard — tabs: Overview, Trades, Logs, Config
  hooks/useOrchestrator.ts Polling hook for stats/positions/closed
  lib/api.ts              API client
```

**Auth flow:** Nexus SSO (`auth.vylth.com`) → Nexus access_token → `POST /auth/nexus` → Hummingbird JWT stored in localStorage as `hb_token`.

---

## Known issues / pending work

1. **QuickNode daily limit** — Free tier exhausted after ~8h of scanning 4 platforms. Consider NodeReal (monthly budget, no daily cap) or QuickNode paid.

2. **raydium_launchlab + boop WebSocket** — QuickNode free tier limits concurrent WSS connections to ~2. With pump_fun + moonshot already connected, raydium and boop connections get dropped. Need paid RPC or reduce platforms.

3. **Scorer social check** — Currently a stub returning 0. Twitter/Telegram lookup not implemented.

4. **Scalper mode** — Not fully implemented. Entry threshold routing works but the second-wave detection logic is basic.

5. **Dev wallet sell detection** — Current implementation counts transactions on the mint account post-entry (>3 = flag). Not parsing actual token transfers. Should use getTokenAccountsByOwner for the dev's wallet.

6. **Min balance SOL** — `min_balance_sol` field is stored in UserConfig but not yet enforced in Trader before entry. Should check `t.Balance() > userCfg.MinBalanceSOL` before `t.enter()`.

7. **Listener Rust binary** — Cross-compiled on dev, rsynced to server. No CI for this yet.

---

## Git commit style

- One-liner messages, no body, no Co-Authored-By
- Example: `git commit -m "fix bonding curve account index for pump_fun"`

---

## Trading wallet (Isaac's)

- Wallet ID: `2f5f5252-f596-48ef-ad0b-42e07661d121` (Signet)
- Needs funding to start trading — small test amount first
- Check balance via dashboard wallets tab or `GET /wallets`


---

## Tasks

Active tasks are tracked via the Labs directives system and registered centrally.

**"Do we have any tasks today?"**

1. Read `/mnt/vylth/labs/TASKS.md` FIRST — This is the central registry of all pending work.
2. Find the section for this specific project.
3. If there is a task marked `[ ] Pending`, see the path it points to.
4. Open the directive file located at that path (e.g. `/mnt/vylth/labs/directives/.../DIR-XXXX-title.md`).
5. Execute the work as instructed in the directive, and check off the boxes inside the directive file.
6. **CRITICAL**: Once the directive is fully complete, you MUST go back to `/mnt/vylth/labs/TASKS.md` and change the status from `[ ] Pending` to `[x] Done`.

Always remember: if you create a new directive yourself, you must register it in `TASKS.md` immediately!