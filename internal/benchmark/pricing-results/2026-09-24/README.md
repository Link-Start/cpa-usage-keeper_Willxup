# Persisted Pricing Scale Results: 2026-09-24

[中文版](README.zh.md)

This directory contains first pricing migrations and 30-day recalculations of **1 million and 10 million synthetic `latest` hot/cold events**, plus a real old/new HTTP query comparison performed only at the 1m scale. A separate **first formal 1m `five-dim` run failed** when its inbox buffer dropped messages; a complete rerun on the same generated dataset passed after reducing historical replay page sizes. Both records are retained: [latest 1m](latest-1000000.json), [latest 10m](latest-10000000.json), [five-dim failure](five-dim-1000000-before-fix.json) and [passing rerun](five-dim-1000000.json), [1m 119-day HTTP comparison](query-comparison-1000000.json), and [environment](environment.json).

## Input and reproduction

The `geelinx-my` Linux/amd64 host had 6 logical CPUs and approximately 9.49 GiB of host memory. Free disk space at generation start was **24.98 GiB for latest 1m** and **31.92 GiB for latest 10m**. All formal tasks had a 7 GiB cgroup memory limit, 512 MiB swap limit, and no CPU quota; `GOMEMLIMIT=5GiB` and `TZ=UTC` were set. The builds used Go 1.26.2, `go-sqlite3 v1.14.48`, and the `sqlite_trace` tag. The product baseline was `4e41adc293f10469e4523a300432ace8b90dfc05`; the fixed build additionally changes two historical migration page sizes, and its binary hash is recorded separately in the environment file. Host memory in the reports is distinct from the cgroup limit.

Both runs used fixed anchor `2026-09-23T00:00:00Z`, seed `20260923`, generator `production-v10-pricing-storage`, 500 identities, 50 API keys, 50 models, **50 actual price rules**, and a 1% failure rate. The 120-day inputs comprised 750,000 hot / 250,000 archived events for 1m and 7.5 million hot / 2.5 million archived events for 10m. Hot and archive windows covered 90 and 30 days; the recent 30-day target was 250,000 or 2.5 million events respectively. The dedicated builder first generated and aggregated data with the existing capacity generator. It then physically removed the new pricing columns and control tables and reverted the pricing migration version in a dedicated database file. It checked old columns, hot/cold counts, tokens, aggregate request counts, and cursor before timing the real first upgrade. Preparation includes generation and conversion to the old schema and is **excluded** from M1–M6 pricing migration timing. Earlier five-dimension/index migrations were already applied in this `latest` input, so its upgrade timing does not measure pending historical five-dimension migration or index creation.

From the repository root, set the size argument in the command below to `1000000` or `10000000` and run them **sequentially** on isolated Linux tasks with the recorded resource constraints. Use a fresh output directory each time. The script runs only the selected scenario and size; it retains the synthetic database, M1 backup, report, and environment inventory without automatic cleanup. Allow disk space for generation, the database, WAL, and backup.

```bash
export TZ=UTC GOMEMLIMIT=5GiB
export PRICING_BENCHMARK_REVISION=4e41adc293f10469e4523a300432ace8b90dfc05
internal/benchmark/scripts/run-pricing-benchmark.sh <new-output-dir> 1000000 latest 2026-09-23T00:00:00Z
```

The upgrade calls the real M1 backup and M1–M6 migration functions. Recalculation uses the real pricing service and repository, writes to the real SQLite inbox while work proceeds, and queries through the product statistics routes. Both formal `latest` runs invoked an already-built benchmark binary directly; the script above provides a repeatable entry point with the same parameters. `PRICING_BENCHMARK_REVISION` only records the product revision in the environment file; it does not pin source code. Separate source-file and binary hashes identify the CMT34 tool used for the measured runs.

## `latest` scale comparison

| Measure | 1m | 10m |
| --- | ---: | ---: |
| Prepare data and old physical schema; excluded from migration | 350.413 s | 2,460.704 s |
| M1–M4 / M5–M6 | 180.720 / 220.191 s | 1,807.565 / 1,765.457 s |
| **M1–M6 pricing migration complete** | **400.913 s; about 2,494 source events/s** | **3,573.025 s; about 2,799 source events/s** |
| 30-day target / actually recalculated | 250,000 / 250,000; 149.507 s | 2,500,000 / 2,500,000; 1,301.036 s |
| Recalculation throughput | About 1,672 rows/s | About 1,922 rows/s |
| Inbox catch-up | 27,522 events; 4.071 s | 243,703 events; 35.429 s |
| Original token total before migration / after migration / after recalculation | 2,696,079,191 throughout | 26,986,552,473 throughout |
| NULL cost rows after migration / unexpected responses during recalculation | 0 / 0 | 0 / 0 |

The M1–M6 clock stops at `CompleteLegacyPricingData`. It excludes subsequent verification, remaining database migrations, and service initialization; it is not a full HTTP startup-to-`ready` time. Both recalculation tasks ended `completed`, and hot/cold event, token, and aggregate request counts passed validation.

## One-million-event results

An independent producer offered one synthetic message every 20 ms to a bounded 4,096-message test buffer; one worker called the actual durable inbox writer. During migration, **20,046 offered / 20,046 committed**; during recalculation, **7,476 / 7,476**. Both stages had zero buffer drops and write errors, with peak buffered counts of 13 and 10. The migration-stage call-to-commit p95/p99/maximum latencies were **53.50/86.84/265.94 ms**, and offer-to-commit p95 was **65.48 ms**. Recalculation values were **7.94/164.96/218.48 ms** and **138.05 ms**. These show durable reception and buffering at roughly 50 messages/s; they are neither CPA network end-to-end latency nor a maximum sustainable ingestion rate. Stored inbox row counts and hashes were checked on disk. Business-event processing occurred during the later catch-up.

| Resource or writer observation | First upgrade | 30-day recalculation |
| --- | ---: | ---: |
| Process peak RSS | 605.9 MiB | 639.7 MiB |
| Sampled peak WAL | 35.58 MiB | 66.48 MiB |
| Sampled peak database + WAL/SHM + M1 backup | 1.678 GiB | 1.708 GiB |
| Longest sampled continuous `InUse` span on the single writer connection | 1,799.9 ms | 950.0 ms |
| Writer-pool wait count / summed wait duration | 2,103 / 111.39 s | 357 / 49.57 s |
| SQLite-traced transactions; maximum write upper / lower bound | 22,053; 205.87 / 204.81 ms | 7,825; 214.20 / 213.62 ms |

Resource sampling ran every **50 ms**, so shorter peaks can be missed. Disk totals include the M1 backup. The upgrade cgroup `memory.peak` was about **2.79 GiB**, including preparation and charged file cache that are not represented by the phase RSS figures. `InUse` is connection occupancy; summed pool waits can accumulate across concurrent requests. Neither is exact SQLite write-lock time. The trace **upper bound** starts at the first write statement and ends after COMMIT/ROLLBACK finishes, so it may include waiting for the first write. The **lower bound** starts after the first successful write finishes and ends when the commit statement starts, excluding the commit tail. The listed maximum upper and lower values can come from different transactions and must not be treated as the endpoints of one exact lock interval. Autocommit writes were counted separately. No active competing-lock probe was run.

During recalculation, the real statistics route returned **1,485 `costs_busy`** responses and zero unexpected responses: cost reads were gated. The runner's single Overview and Analysis queries over the 120-day synthetic dataset took 100.38 and 938.44 ms after upgrade but before recalculation, and 67.53 and 950.19 ms after recalculation plus inbox catch-up. These single calls are **not an old-version performance baseline**.

## Ten-million-event results

M1–M4 included the real backup and event cost backfill; M5–M6 completed aggregate costs and full validation. The final Overview cursor was 10,000,000, with zero NULL cost rows. The actual 30-day target and processed count were both 2,500,000, completed in 1,301.036 seconds. Each of the 243,703 retained messages then produced one event; catch-up throughput was about **6,879 events/s**. The runner elapsed about **7,393.766 seconds** from start to finish, including 2,460.704 seconds of preparation excluded from migration timing.

Both stages continued to offer an independent message every 20 ms to the same 4,096-message test buffer and real inbox writer. Offered, committed, and disk-verified counts were each **178,651** during migration and **65,052** during recalculation. Both stages had zero drops and write errors; peak buffered counts were 11 and 18. Migration call-to-commit p95/p99/maximum were **43.68/61.36/225.75 ms**, with offer-to-commit p95 **45.93 ms**. Recalculation values were **6.12/134.88/378.72 ms** and **112.00 ms**. This still validates durable reception only at roughly 50 messages/s, not maximum ingestion capacity.

| 10m resource and writer observation | M1–M6 | 30-day recalculation |
| --- | ---: | ---: |
| Process peak RSS | 1,442.6 MiB | 992.6 MiB |
| Sampled peak WAL | 262.52 MiB | 282.08 MiB |
| Sampled peak database + WAL/SHM + M1 backup | 12.908 GiB | 12.927 GiB |
| Longest sampled continuous `InUse` span on the single writer connection | 1,600.3 ms | 949.9 ms |
| Writer-pool wait count / summed wait duration | 20,963 / 858.45 s | 2,793 / 338.17 s |
| SQLite-traced transactions; maximum write upper / lower bound | 198,658; 182.44 / 180.79 ms | 67,773; 183.82 / 183.33 ms |

The 10m upgrade task's cgroup `memory.peak` was about **6.85 GiB**, including preparation and charged file cache, close to the 7 GiB limit; phase RSS is not the cgroup peak. Resource sampling remained at 50 ms and disk totals include the M1 backup. The write-bound, connection-occupancy, and pool-wait definitions above also apply. Both traces had zero incomplete transactions. During recalculation the real statistics route returned **12,914 `costs_busy`** responses and zero unexpected responses. The runner's single Overview and Analysis queries over the 120-day synthetic dataset took 175.37 and 1,587.91 ms after upgrade but before recalculation, then 102.35 and 1,362.01 ms after recalculation plus inbox catch-up. These are not an old-version baseline; this directory contains no 10m old/new HTTP comparison.

## 1m `five-dim` failure and passing rerun

The [failed run](five-dim-1000000-before-fix.json) and [passing rerun](five-dim-1000000.json) used the same generated fingerprint `5c42fe20…`: one million all-hot events, 50 price rules, an independent arrival every 20 ms, and a 4,096-message test buffer. This scenario had pending old five-dimension, checkpoint, Latency, and pricing migrations. Its aggregate M1–M6 time cannot be attributed to any one of those components. The first run reached M6 `data_complete`, but 2,620 dropped messages failed the complete-reception check; **recalculation and catch-up did not run**. Zero values in that failure report are unmeasured fields.

| Observation | First run failed | Complete pass after 1,000→100-row pages |
| --- | ---: | ---: |
| Total M1–M6 pricing migration time | 547.992 s | **772.901 s**, 224.909 s longer |
| Migration inbox offered / committed / disk-verified | 27,400 / 24,780 / not run | **38,645 / 38,645 / 38,645** |
| Buffer drops / peak queued | 2,620 / 4,096 | **0 / 1,300** |
| Maximum single write call | 375.876 ms | **139.115 ms** |
| Maximum offer-to-commit delay | 134.339 s | **26.001 s** |
| Maximum SQLite-traced single-transaction write upper bound | 354.88 ms | **155.24 ms** |

The fix traded a longer total migration time for zero drops. The **26.001 s** maximum offer-to-commit delay still shows backlog; reception was neither immediate nor wait-free. In the first run, old five-dimension replay took about 130 s and initial Latency replay about 45 s; many short transactions gave the receiver too few writing opportunities, rather than one very long SQLite write lock. The passing run's longest sampled continuous writer-connection `InUse` span was 409.25 s, versus 176.90 s before the fix; this is connection occupancy, not lock duration. Both runs had about 35 MiB peak WAL and 1.56 GiB sampled peak disk including the M1 backup. Their cgroup `memory.peak` values were about 2.75 and 2.78 GiB respectively, so resource use remained on the same scale.

The passing rerun recalculated **333,333 target / 333,333 processed** rows in **188.363 s**, ending `completed`. Its recalculation inbox had another **9,419 offered / 9,419 committed / 9,419 disk-verified** messages, with zero drops and write errors. A single catch-up processed **48,064** messages across both stages into events in 7.072 s. Post-upgrade NULL cost rows were zero; the original token total stayed **2,696,079,191** before migration, after migration, and after recalculation. The real statistics route returned **1,870 `costs_busy`** responses and zero unexpected errors during recalculation. The fixed binary hash and cgroup peak are in the [environment](environment.json); source inventories for the [failed](five-dim-1000000-before-fix-source.sha256) and [passing](five-dim-1000000-source.sha256) runs are retained.

## Old/new 119-day HTTP comparison

The old product binary corresponds to `54a43cb324f33c1039c7622d842e005257a68466`; the new binary corresponds to `4e41adc…`. The comparison script made separate SQLite copies of the **old M1 backup** and upgraded database, started the two real Keeper binaries sequentially with a local synthetic CPA returning 404, warmed each endpoint once, then made five HTTP requests per endpoint. The fixed query was `range=custom&unit=day&start=2026-05-26&end=2026-09-21`, covering **119 days**. It deliberately excludes the final day containing messages injected ten minutes before the anchor so the databases share the same request and token facts. Both Overview and Analysis returned **991,361 requests and 2,672,835,362 tokens** on both versions. Dollar amounts were not compared because the pricing formulas differ between versions.

| HTTP route | Old median, 5 samples | New median, 5 samples | Observation |
| --- | ---: | ---: | --- |
| Overview | 1,301.07 ms | **71.02 ms** | About 18.3 times faster at the median |
| Analysis | 1,706.19 ms | 1,757.33 ms | About 3.0% slower at the median; five samples cannot establish a stable regression |

The [comparison JSON](query-comparison-1000000.json) preserves all five samples and min/max values. To repeat it, build old and new product binaries from the revisions above, then supply the M1 backup and upgraded database from the first-upgrade run. The script clones each source into a **new** comparison directory without changing the inputs:

```bash
python3 internal/benchmark/scripts/compare-pricing-queries.py \
  --old-binary <old-binary> --new-binary <new-binary> \
  --backup <M1-backup.db> --database <upgraded-pricing-old.db> \
  --root <new-comparison-dir> --anchor 2026-09-23T00:00:00Z --samples 5
```

This is one short, sequential comparison on one host, without concurrent users, successful CPA responses, repeated runs, or confidence intervals. It supports observations only for these routes and this matched fact set, not all queries or general ingestion performance.

## Source inventory and remaining coverage

The **seven runner Go files** in the [1m](latest-1000000-source.sha256) and [10m](latest-10000000-source.sha256) startup inventories have identical SHA-256 hashes matching the current worktree. The benchmark binary hash is in the [environment record](environment.json). The 1m inventory hash for `run-pricing-benchmark.sh` differs because the 1m run directly invoked its built binary and the shell script was subsequently changed to use portable cache paths and a supplied revision; that change did not alter the recorded runner code or 1m result. The 1m inventory also predates the HTTP comparison script's change from a range including the injected-message day to the present 119-day range excluding that day. The script actually used for the HTTP comparison had SHA-256 `f58ad822d8151dd92da06afe732cc04d9dcf28298f07df8bdf043c50649a2130`, matching the current file. Thus the 1m inventory is a **startup snapshot**, not a snapshot of the later comparison script; the 10m inventory contains only the runner Go files.

Neither `latest` run exercised pending old five-dimension migration or its long writes; the separate fixed 1m `five-dim` rerun now covers that scenario. The fix only changed the old five-dimension and Latency replay page limits from 1,000 to 100; the seven runner Go files did not change. Because the `latest` inputs already had these historical migrations applied, their earlier measured results remain applicable. The fixed binary SHA-256 and changed-file hashes are in the [environment](environment.json) and [passing-run source inventory](five-dim-1000000-source.sha256).
