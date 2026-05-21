# Hummingbird — Agent Instructions

## Response Style — Concise by Default

- Lead with the answer. No preamble, no warm-up.
- Full facts, fewer words. Compress the prose, never drop substance — 100% of the facts, none of the padding.
- Tight bullets and short sentences over paragraphs. No walls of text.
- Push back when warranted — one sharp point, not an essay.
- Cut: restating the question, recapping what you just did, hedging, filler transitions.
- If it can be a list or table, make it one. One scannable block beats three paragraphs.

---

Autonomous pump.fun trading agent. Rust + Python + Go.
**Repo:** https://github.com/iamdecatalyst/hummingbird (public)
**Strategic purpose:** Open source viral project → drives Signet API signups → VYLTH revenue.

---

## Before Starting Any Work

Read `/mnt/vylth/labs/operations/known-patterns.md` — cross-ecosystem anti-patterns and known bug patterns. These are real mistakes that have occurred across Vylth repos. Read them before touching any code.

If you encounter a bug, anti-pattern, or recurring mistake during your work that is not already listed there, add it to `known-patterns.md` before finishing. Include the pattern category, what went wrong, and the correct approach.

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
JWT_SECRET=<at least 32 chars — orchestrator log.Fatals if unset/default in multi-tenant>
SCORER_SECRET=<at least 32 chars — same value in scorer + listener .env; auths /trade and /score>
ALLOWED_ORIGINS=https://hummingbird.vylth.com[,...]  # CORS allowlist; '*' rejected in multi-tenant
TELEGRAM_TOKEN=<bot token>
RPC_HTTP=https://divine-frequent-spree.solana-mainnet.quiknode.pro/<key>
RPC_WS=wss://divine-frequent-spree.solana-mainnet.quiknode.pro/<key>
SOLANA_RPC=https://divine-frequent-spree.solana-mainnet.quiknode.pro/<key>
SIGNET_BASE_URL=https://api.signet.vylth.com/v1
```

**RPC note:** QuickNode free tier has a **daily HTTP request limit** AND limits concurrent WSS to ~2. Cannot launch on free tier — upgrade to NodeReal monthly or QuickNode Build/Scale before opening to users.

**Deploy gotcha — go.mod replace path:** committed `go.mod` uses `replace github.com/VYLTH/signet-sdk-go => /mnt/vylth/signet/signet-sdk-go` (dev path). Production has signet at `/opt/signet/signet-sdk-go`. After every `git pull`, run `sed -i "s|/mnt/vylth/signet|/opt/signet|" /opt/hummingbird/orchestrator/go.mod` before rebuilding. (TODO: replace with go.work overlay.)

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

1. **QuickNode RPC limits** — Both daily HTTP cap AND concurrent WSS ≤ 2. **Launch blocker**. Upgrade to NodeReal (monthly, no daily cap) or QuickNode Build/Scale before opening to users.

2. **Scorer social check** — Currently a stub returning 0. Twitter/Telegram lookup not implemented. Not a launch blocker; just leaves 10pts on the table.

3. **Scalper mode** — Entry routing works but second-wave detection logic is basic (only Cricket Firefly "accumulation" signal, hardcoded 0.05 SOL position).

4. **Dev wallet sell detection** — Now uses Cricket Firefly exodus signals rather than naive tx counting. Not perfect but adequate.

5. **Listener Rust binary** — Cross-compiled on dev, rsynced to server. No CI for this yet.

6. **Per-user sniper/scalper toggles ignored** — `SniperEnabled` / `ScalperEnabled` are stored in UserConfig but the dispatch path always fans out to all users (audit H3). Cosmetic UX bug, not fund-loss.

7. **DexScreener rate limits** — every monitor polls DexScreener every 2s. With multiple users × multiple positions, public API gets throttled (audit H2). Need a shared rate-limited client + cache.

8. **Listener has no buffered retry** — if scorer is briefly down, detected tokens are silently dropped (audit H12). Add a small bounded retry queue.

## Recently fixed (pre-launch hardening, 2026-04-18)

- B1 — `/trade` + `/score` now require `Authorization: Bearer <SCORER_SECRET>` (was unauthenticated drain-everyone vector)
- B2 — per-user `MaxPositionSOL` is enforced in `Trader.Execute` (was cosmetic)
- B3 — entry price is now `positionSOL / actual UI token balance` after swap (was the lamport notional, causing instant-stop-outs)
- B4 — pumpportal-returned txs are parsed and validated (no SystemProgram, fee payer matches wallet)
- B5 — `/wallets/{id}/withdraw` has per-call cap (10 SOL), per-day cap (50 SOL), destination format check, ownership check
- B6 — orchestrator `log.Fatal`s if `JWT_SECRET` is missing/default/short in multi-tenant; CORS now uses `ALLOWED_ORIGINS` allowlist (no `*`)
- B7 — monitor exit signal is now blocking (with 5s warn) so SL/rug signals can't be silently dropped
- H1 — `take_profit_level` + `peak_price` persisted on every TP advance (no more re-firing TP1/TP2 on restart)
- H6+H7 — long-running goroutines wrapped in `util.Go(label, fn)` with panic recovery; `util.ShortMint(s)` helper for safe slicing
- H8 — exit P&L retries Balance fetch 3x; reports honest loss (with telegram alert) if RPC stays broken instead of fake breakeven
- H10 — JWT TTL shortened from 30d → 24h
- min_balance check now buffers ~0.005 SOL for slippage + priority fee + ATA rent

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
## Nexus Auth Docs

To fetch the Nexus SDK integration guide from the command line:

```bash
curl https://auth.vylth.com/api/nexus/docs.txt
```

Covers: domains, SDKs, roles, frontend/backend auth, JWT claims, Mail API (user + app keys), service auth, image upload.

---

## Loaders

This frontend uses **loading-ui.com** components via the shadcn registry for every loading state — spinners, skeletons, progress, shimmer. Do not hand-roll CSS spinners and do not pull from generic libraries.

**Install on demand, per component** (never bulk):

```bash
npx shadcn@latest add @loading-ui/<component>
# Older shadcn-ui projects:
# npx shadcn-ui@latest add @loading-ui/<component>
```

**Pick the right loader for the context:**

| Context | Component |
|---|---|
| Image upload / analysis / generation | `analyzing-image` |
| LLM streaming / model thinking | `text-shimmer`, `text-shimmer-wave`, `typing` |
| Content placeholder (cards, lists, tables) | `skeleton` |
| Terminal / log / CLI output | `terminal` |
| Generic async (button, inline, page) | `ring`, `dots`, `pulse`, `bars` |
| Long-running multi-step job | `orbit-ring`, `concentric-ring`, `infinity` |
| Playful / mascot / empty state | `wandering-eyes`, `bobbing-dots` |

Full catalog and decision matrix: [`labs/arsenal/loading-ui.md`](/mnt/vylth/labs/arsenal/loading-ui.md). Source: https://loading-ui.com/.

Wire colour to the per-app accent token (Tailwind class) for on-brand output.

---

## Editorial Documents (Whitepapers, Pitch Decks, Revenue Models)

When asked to create a **whitepaper, pitch deck, revenue model, strategy paper, investor doc, product spec, or any shareable HTML document that is NOT an app UI** — use the Vylth whitepaper style. Do not hand-roll an editorial layout.

- **Style guide:** [`/mnt/vylth/labs/design/whitepaper-style.md`](/mnt/vylth/labs/design/whitepaper-style.md) — read first.
- **Template:** [`/mnt/vylth/labs/design/whitepaper-template.html`](/mnt/vylth/labs/design/whitepaper-template.html) — copy and fill in.
- **Reference:** [`/mnt/vylth/vylth-flow/docs/flow-revenue-model.html`](/mnt/vylth/vylth-flow/docs/flow-revenue-model.html) — the original document the style was extracted from.

**Style summary:** white background, black text, JetBrains Mono for all numbers, Inter for prose, 1–2px black borders, one inverted (black) hero card per doc max, print-ready. Distinct from the neumorphic dark app UI in `labs/design/theme.md`.

**Default location** for new editorial documents in this repo: `docs/`.

**Filename conventions:**

| Type | Filename |
|---|---|
| Revenue model | `[product]-revenue-model.html` |
| Pitch / investor | `[product]-pitch-deck.html` |
| Product spec | `[product]-spec-[feature].html` |
| Strategy paper | `[product]-strategy-[topic].html` |
| Whitepaper | `[product]-whitepaper.html` |

Anti-patterns: gradients, rounded corners on structural elements, shadows, colored text, multiple inverted cards.

---

## React Doctor

This frontend is scored by **react-doctor** — Million.co's React code-health scanner. Run before declaring any feature done:

```bash
npx -y react-doctor@latest .
```

You'll get a 0–100 score across state & effects, performance, architecture, security, accessibility, and dead code. Rules toggle automatically based on framework (Next.js / Vite / React Native) and React version.

**Vylth rule:** target ≥75. A merged PR must not lower this repo's score.

**Install rules into your agent (once per workstation):**
```bash
npx -y react-doctor@latest install
```

This teaches Claude Code / Cursor / Codex the same rules so they stop introducing the issues in the first place.

**Baseline score:** 78 / 100 (Great) — recorded 2026-05-09.

Reference: [`labs/arsenal/react-doctor.md`](/mnt/vylth/labs/arsenal/react-doctor.md). Source: https://react.doctor.


<!-- VYLTH-DESIGN-SKILLS:BEGIN (managed block — safe to regenerate, do not hand-edit) -->
## Design & Asset Skills

User-level Claude Code skills auto-trigger on these task types. If you are explicitly doing one of these, invoke the skill by name — don't hand-roll it. Each skill carries non-negotiable rules, a mandatory build order, anti-patterns, and a quality checklist. Visual treatment defers to `premium-glass-ui`; motion to `web-motion`/`remotion-premium-video`; raster rendering to `nano-banana`; so output stays consistent across sessions.

| When doing… | Invoke skill |
|---|---|
| Glassy / premium / sleek / 3D-depth UI | `premium-glass-ui` |
| Tactile hardware-style controls (knobs, dials) | `skeuomorphic-ui` |
| UI animation / button & hover / micro-interactions | `web-motion` |
| Forms / sign-up / checkout / validation / multi-step | `form-ux` |
| "Where do I find UI/design/template references" | `ui-design-sources` |
| Card component (stat / quote / pricing / dashboard tile / OG share) | `card-primitives` |
| Remotion video / product ad / onboarding video | `remotion-premium-video` |
| A single video frame or thumbnail/preview frame | `video-frame-composition` |
| Landing / marketing / pricing page | `premium-landing-page` |
| Swipe carousel (X / Instagram / LinkedIn) | `social-carousel` |
| OG / link-preview / social share image | `og-image` |
| Pitch / investor / fundraising deck slide | `pitch-deck-slide` |
| Infographic / data snapshot / stat graphic | `infographic-design` |
| App Store / Play Store listing screenshots | `app-store-screenshots` |
| Brand meme | `brand-meme` |
| Wallpaper / lockscreen / home-screen widget | `wallpaper-widget` |
| Smart-contract / DeFi protocol security audit | `smart-contract-audit` |
| Backend blockchain integration (RPC/tx/reorg/indexing) | `onchain-integration` |
| Generate/ render an actual image (Gemini image gen) | `nano-banana` |
<!-- VYLTH-DESIGN-SKILLS:END -->
