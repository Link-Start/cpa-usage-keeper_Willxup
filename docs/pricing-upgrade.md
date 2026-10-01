# Pricing and upgrade guide

[简体中文](pricing-upgrade.zh.md)

Keeper now calculates a USD total when each CPA usage event is first stored. Hourly and daily summaries use that stored event cost. Editing or deleting a model price, or applying prices from a source, changes future pricing; it does not silently rewrite historical costs. Use an explicit recalculation when older stored events should use the current configuration.

## Configure model prices

A model configuration has a `model`, `pricing_style` (`openai` or `claude`), four `base_prices` (`input`, `output`, `cache_read`, `cache_write`), a `model_multiplier`, `conditional_multipliers`, and `branches`. Prices are in **USD per million tokens**. A complete `PUT /api/v1/pricing/models` must include every field; `[]` explicitly clears rules or branches. An explicit zero price is valid and is distinct from an unavailable price.

For each event, Keeper matches the actual model first, then its alias. A matching branch replaces the four base unit prices for the **whole request**; otherwise, the base prices apply. Branch context uses normalized `input_tokens` (`all`, `gt`, `lte`, or `range`). Its daily period is `all` or an `HH:mm` window evaluated against the stored CPA event time in the deployment time zone; the start is inclusive, the end exclusive, and a window may cross midnight. Overlapping branch conditions are rejected. The model multiplier and all matching conditional multipliers then apply to the whole request. A missing model price for an event that needs pricing gives a zero amount with `cost_available: false`; an explicitly free price remains available.

`GET /api/v1/pricing/sync/fetch?source=models-dev` (or `litellm`) only returns candidate matches for review. `POST /api/v1/pricing/sync/apply` accepts selected items with `model`, `pricing_style`, and four `base_prices`. For an existing model, application updates **only the four base prices**: its saved pricing style, model multiplier, conditional multipliers, and branches remain. A newly added model starts with the source style, multiplier `1`, and empty rules and branches. Applying a source does not recalculate stored events.

## Recalculate historical costs

The Pricing page offers a manual recalculation for **all models** in a chosen recent hot-event interval; it is not a per-model operation. First read `GET /api/v1/pricing/recalculations/options`. The server returns its deployment time zone, legal absolute-hour start bounds, a 3,600-second step, a 30-day maximum, and the current `config_revision`. Submit the selected `start_at` as an ISO 8601 instant with its offset, together with that revision, to `POST /api/v1/pricing/recalculations`. The server checks the bounds and revision again when accepting the request and fixes the end time then. The processed interval is `[start_at, end_at)`; archived events outside the hot window are not a target.

Only one task runs in a process. A second start while it is running returns the existing task with `started: false`; it does not queue another. Poll `GET /api/v1/pricing/recalculations/current` for `running`, `completed`, or `failed` and committed-row progress. The task is held in memory: after a process restart, `current` returns `null`, and there is no automatic task resume. Each committed page updates event, hourly, and daily costs together. If the task fails or the process stops, committed pages remain; after fixing the cause, start a **new** recalculation for the desired range rather than assuming the old task will resume. Re-running replaces costs and reconciles summary differences; it does not add request or token counts again.

During recalculation, affected cost-reading APIs return HTTP `503` with code `costs_busy`; price configuration writes return HTTP `409` with code `pricing_busy`. Keeper can still durably receive new CPA messages into its inbox while event processing and ordinary aggregation are paused. Those messages are processed after work resumes. The options and current-task endpoints remain available for progress checks.

## First upgrade of an existing database

Keep the existing database and its backup directory available and allow space for a complete M1 backup, WAL, and migration work. On the first startup with an old database, Keeper creates and verifies one protected **M1 backup before any destructive historical migration**. It then runs any pending published schema migrations, fixes the existing hot/cold event and aggregation boundaries, and backfills stored event costs and hourly/daily totals in committed pages. It validates the pricing data before exposing the full business API. New empty databases take the fresh-schema path and do not need this legacy backup.

The startup shell remains accessible during migration. Check `GET /api/v1/startup/status` for `opening`, `migrating`, `failed`, or `ready`; `/healthz` alone does not prove the business API is ready. A failed phase requires investigation before continuing. Do not start an older Keeper binary on a partially upgraded or newly upgraded database.

### Recover after a failed first upgrade

The offline `pricing-restore` tool is for a **failed legacy first upgrade with its valid protected M1 backup**. It creates a new old-schema database from that backup and replays inbox messages that reached the fault database after the backup. It is not a general downgrade or an ordinary scheduled-backup restore.

1. Stop Keeper and every other writer to the fault database. Preserve the fault `app.db` **with its `-wal` and `-shm` sidecars**, and keep the original M1 backup at its recorded path. Do not copy only the fault main file, remove its sidecars, or move the M1 backup: the tool checks backup identity against the fault database's protection state.
2. From this source tree, build the tool and choose a **new** output directory. Replace the example backup path with the actual recorded M1 filename. The output `app.db` and its sidecar names must not already exist:

   ```bash
   go build -o ./pricing-restore ./cmd/pricing-restore
   mkdir -p ./recovery
   ./pricing-restore \
     -backup "./old-work/backups/M1-backup.db" \
     -fault "./old-work/app.db" \
     -out "./recovery/app.db"
   ```

3. Check the printed M1 inbox maximum ID, fault inbox maximum ID, and replayed-row count. The tool checks SQLite integrity and the recorded M1 boundary, then copies every fault inbox row with an ID **after the backup's actual maximum** through the fault maximum. It preserves each row's ID, raw message, hash, and receipt times without content deduplication. Replayed rows, including ones marked processed in the fault database, become pending because their events are absent from the restored backup. The backup and fault files are not overwritten.
4. Keep the old work directory untouched. Configure a separate `WORK_DIR` containing the newly created `app.db` (and a separate backup directory if customized), then start the **current** Keeper version on it. Wait for `/api/v1/startup/status` to report `ready`, check usage and inbox processing, and retain the original fault database, its sidecars, and the M1 backup until recovery is verified. Do **not** connect an old binary directly to the upgraded database or use this restored file as a completed upgrade; the current version must perform its migration again.

If the output path or one of its sidecar paths already exists, or the M1 file does not match the fault database, the tool refuses to publish a result. Use a fresh output path after correcting the input; do not overwrite the evidence files.

## Pricing API compatibility

All paths below are under Keeper's `/api/v1` (and any configured `APP_BASE_PATH`). Pricing routes require administrator access.

| Method and path | Contract |
| --- | --- |
| `GET /pricing/models` | Full editable model configurations and `config_revision` |
| `GET /pricing/model-options` | Candidate model names |
| `PUT /pricing/models` | Replace one complete model configuration; return new revision |
| `DELETE /pricing/models?model=...` | Remove current configuration; return new revision |
| `GET /pricing/sync/fetch?source=...` | Review candidate base prices from `models-dev` or `litellm` without saving |
| `POST /pricing/sync/apply` | Apply `source` and selected `items[]` with `model`, `pricing_style`, and four `base_prices` |
| `GET /pricing/recalculations/options` | Server-computed start hours and revision |
| `POST /pricing/recalculations` | Start using `start_at` and `config_revision` |
| `GET /pricing/recalculations/current` | Current in-process task, or `null` |

The previous `GET`/`PUT`/`DELETE /pricing`, `PUT /pricing/:model`, `GET`/`PUT /pricing/rules`, `PUT /pricing/batch`, `GET /pricing/sync/preview`, and `GET /models/used` routes were removed and return `404`. API clients must use the full model, sync, and recalculation contracts above.

The Analysis response replaces `cost_breakdown` with `cost_summary`, containing only `total_cost_usd` and `cost_available`. The former `uncached_input_cost_usd`, `cache_read_cost_usd`, `cache_write_cost_usd`, and `output_cost_usd` fields are gone. Clients reading the old object or fields must update; token counts remain available in the regular Analysis data.

## Measured scale and limits

The [pricing benchmark results](../internal/benchmark/pricing-results/2026-09-24/README.md) include reproducible 1m and 10m synthetic hot/cold datasets, a separate historical five-dimension migration run, and a matched 119-day old/new HTTP query comparison at 1m. On the recorded 6-vCPU Linux host, measured M1–M6 pricing migration took **400.913 seconds at 1m** and **3,573.025 seconds at 10m**; the 10m task reached **6.85 GiB cgroup memory peak** and **12.927 GiB sampled database/WAL/backup peak**. Reducing the older five-dimension and Latency replay pages from 1,000 to 100 eliminated drops at the tested ~50-message/s inbox load, while the five-dimension M1–M6 run took longer and still showed up to **26.001 seconds** of offer-to-commit delay. These are observations for the documented hardware, data mix, and rules, not minimum resource requirements or performance guarantees for other configurations.
