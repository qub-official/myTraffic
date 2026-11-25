# myTraffic roadmap — toward a full free Burp/Caido alternative

This maps every major Burp Suite and Caido feature (from their official docs) to
myTraffic's status, so the project has a clear, honest path. Legend:
**✅ done** · **🟡 partial** · **⬜ planned**.

Goal: everything that Burp and Caido put behind a paid tier is free and open
source here, running fully locally with no account and no servers.

---

## Core proxy & traffic

| Feature | Burp / Caido name | Status |
|---|---|---|
| HTTP/HTTPS intercepting proxy | Proxy / Intercept | ✅ done |
| On-the-fly TLS CA, per-host leaf certs | CA management | ✅ done |
| Traffic history, live feed | HTTP history | ✅ done |
| Manual intercept (edit/forward/drop) | Intercept | ✅ done |
| Match & replace (req/resp, regex, headers/body) | Match and replace / Tamper | 🟡 partial — add per-section targets (path, query, status line), capture groups, WS |
| Scope (host suffixes) | Scope | 🟡 partial — upgrade to HTTPQL-based scope + include/exclude |
| WebSocket history + StreamQL | WebSockets history / StreamQL | ✅ done — frames captured both directions, StreamQL filter (`ws.*`/`stream.*`), live UI tab |
| Invisible / transparent proxying | Invisible proxy | ⬜ planned |
| Upstream proxy chaining | Upstream proxy | ✅ done — route outbound via another proxy (UI + `--upstream`) |
| HTTP/2 support | HTTP/2 | ⬜ planned |

## Query & search

| Feature | Name | Status |
|---|---|---|
| Query language for filtering | HTTPQL | ✅ done — full namespaces (`req`/`resp`/`row`), operators (`eq ne gt gte lt lte cont ncont like nlike regex nregex`), `AND`/`OR`, parentheses, bare-string raw search |
| Saved filter presets | Filters / presets | ✅ done — named queries saved via API/UI |
| Cross-source search | Search | 🟡 partial — filter works; add source tagging |

## Testing tools

| Feature | Burp / Caido name | Status |
|---|---|---|
| Resend/edit single request | Repeater / Replay | ✅ done |
| Fuzzing / automated attacks | Intruder / Automate | ✅ done — attack types Sniper, Battering ram, Pitchfork, Cluster bomb; payloads: list, numbers, dates, brute-force, null, chars; grep-match/extract; concurrency |
| Replay collections / sessions | Replay collections | ⬜ planned |
| Race-condition testing | Replay pipelines | ⬜ planned |
| Response diff | Comparer | ✅ done — word/line diff |
| Token randomness analysis | Sequencer | ✅ done — Shannon entropy, per-position analysis, verdict (UI + CLI) |
| Encode/decode/hash | Decoder / Convert | ✅ done (base64, url, hex, md5, sha1, sha256, jwt) |
| Discovered-content tree | Target site map / Sitemap | ✅ done — host/path tree from history |

## Findings, scanning, OOB

| Feature | Burp / Caido name | Status |
|---|---|---|
| Manual findings / notes | Findings / Organizer | ✅ done — findings list with severity + flow annotations (color, note, star) |
| Passive scanner (heuristics) | Passive scan | ⬜ planned (deferred) |
| Active scanner | Scanner (Pro) | ⬜ planned — big; after passive |
| Out-of-band interactions | Collaborator (Pro) | ⬜ planned — needs a listener; design as optional local OOB server |

## Automation & extensibility

| Feature | Burp / Caido name | Status |
|---|---|---|
| Session handling / macros / cookie jar | Session handling rules | ⬜ planned |
| Visual/script workflows | Workflows | ⬜ planned |
| Plugin/extension runtime | Extensions (Montoya) / Plugins SDK | ⬜ planned — embed a JS engine (goja) for user scripts |
| Scriptable table filters | Bambdas | ⬜ planned |

## Platform

| Feature | Status |
|---|---|
| Web UI, light + dark | ✅ done |
| CLI (`history`, `replay`, `decode`, `automate`, `ca`) | ✅ done |
| MCP server for AI assistants | ✅ done |
| Windows installer | ✅ done |
| Projects: save/load a session to a file | ✅ done — export/import flows, findings, presets, rules, scope, WS (UI + CLI) |
| Config file | ✅ done — `config.json` in the config dir sets defaults |
| macOS / Linux builds | ⬜ planned (code is cross-platform; add build targets) |

---

## Suggested build order

1. **Full HTTPQL** — foundational; everything filters through it. ✅
2. **Automate/Intruder** — flagship, most-used, paywalled elsewhere. ✅
3. **Sitemap + Comparer** — high value, low cost. ✅
4. **WebSocket history + StreamQL** — common gap. ✅
5. **Findings + annotations** — turn captured traffic into results. ✅
6. **Projects (save/load)** + config file + presets + upstream proxy. ✅
7. **Sequencer** — token randomness. ✅
8. **Session handling / macros** — needed for authenticated testing. ⬜ next
9. **Plugin runtime (goja JS)** + workflows. ⬜ deferred (requested later)
10. **Passive then active scanner**, **OOB listener** — heavy specialist tools. ⬜ deferred

Contributions welcome — pick any ⬜ item.
