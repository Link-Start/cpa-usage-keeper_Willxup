# Pricing benchmark

[简体中文](README.zh.md) · [All benchmarks](../../README.md)

**Migration and recalculation completed at 1m and 10m events, with no dropped incoming messages at the tested 50 messages/s.** At 10m, migration took about 60 minutes and a 2.5m-event recalculation about 22 minutes. These are synthetic measurements, not minimum hardware requirements or maximum ingestion capacity.

## Results

| Measure | 1m events | 10m events |
| --- | ---: | ---: |
| Hot / archived events | 750,000 / 250,000 | 7,500,000 / 2,500,000 |
| First pricing migration | 400.913 s | 3,573.025 s |
| Recalculated events | 250,000 | 2,500,000 |
| Recalculation time | 149.507 s | 1,301.036 s |
| Incoming messages, committed and verified | 27,522 | 243,703 |
| Drops / write errors | 0 / 0 | 0 / 0 |
| Backlog processing after resumption | 4.071 s | 35.429 s |
| Whole-task cgroup memory peak | 2.79 GiB | 6.85 GiB |
| Sampled database + WAL/SHM + backup peak | 1.708 GiB | 12.927 GiB |

Migration timing covers the protected backup, event backfill, rollup costs and validation (M1–M6), not full HTTP startup. Dataset preparation is excluded: 350.413 s at 1m and 2,460.704 s at 10m. Both recalculations completed; original event/Token facts were preserved and no cost remained NULL. Memory peaks include preparation and charged file cache, not just process RSS.

## Conditions

- Linux amd64, 6 logical CPUs, 9.49 GiB host RAM; each task limited to 7 GiB memory and 512 MiB swap, with no CPU quota. Go 1.26.2, go-sqlite3 v1.14.48, `sqlite_trace`, UTC.
- 120 days of history: 90 hot and 30 archived; 500 identities, 50 keys, 50 models, 50 rules, 1% failed events. Seed `20260923`, anchor `2026-09-23T00:00:00Z`.
- One independent arrival every 20 ms, a 4,096-message test buffer and the real single-writer SQLite inbox. Counts and message hashes were verified, followed by actual event processing. This is not CPA network end-to-end latency.
- Product baseline `4e41adc293f10469e4523a300432ace8b90dfc05`; the passing historical-migration build adds the two 100-row replay page changes later committed in `94eef02c`. [Environment and binary hashes](results/environment.json) identify the runs.

## Historical upgrade and query comparison

The separate 1m historical-schema run initially dropped 2,620 messages. Reducing the old five-dimension and Latency replay pages from 1,000 to 100 rows removed drops on the same dataset and arrival rate. Total migration increased from 547.992 to 772.901 s; maximum offer-to-commit delay fell from 134.339 to **26.001 s**, so temporary backlog remains. The passing run verified all 48,064 incoming messages and recalculated 333,333 events in 188.363 s. The failed run did not execute recalculation or catch-up; zero fields for those stages are unmeasured.

A separate real-HTTP comparison used old `54a43cb3` and new `4e41adc2` binaries sequentially, one warmup plus five samples per endpoint. Both returned 991,361 requests and 2,672,835,362 tokens over the same 119-day interval:

| Endpoint | Old median | New median |
| --- | ---: | ---: |
| Overview | 1,301.07 ms | 71.02 ms |
| Analysis | 1,706.19 ms | 1,757.33 ms |

Five samples do not establish a stable Analysis regression. This comparison uses a local synthetic CPA returning 404, excludes the injected-message day, and is not a 10m query comparison or a concurrent-user test.

## Reproduce

See the [pricing run guide](../../guides/pricing.md) for prerequisites, scenario commands and the HTTP comparison.

## Evidence and limits

- [1m](results/latest-1000000.json), [10m](results/latest-10000000.json), [historical failure](results/five-dim-1000000-before-fix.json), [historical pass](results/five-dim-1000000.json), [HTTP samples](results/query-comparison-1000000.json).
- Published JSON omits private hostnames; measurements are unchanged. Source inventories describe the original measured files and retain their original checksums.
- Resources were sampled every 50 ms and can miss shorter peaks. SQLite trace values are write-duration bounds, not exact locks; connection occupancy and cumulative pool wait are separate metrics.
- The latest-schema inputs already include old index/five-dimension migrations. Only the separate historical case measures those pending upgrades. No maximum-rate test, long soak, repeated confidence analysis or production database migration is claimed.
