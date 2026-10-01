# 价格与升级指南

[English](pricing-upgrade.md)

Keeper 现在会在首次保存 CPA 用量事件时计算并保存一笔美元总费用，小时、日汇总使用该已存事件费用。编辑或删除模型价格、应用外部来源价格，只影响后续计价，不会悄悄改写历史费用。如需让已存事件按当前配置计价，须明确启动历史重算。

## 配置模型价格

一份模型配置包含 `model`、`pricing_style`（`openai` 或 `claude`）、四项 `base_prices`（`input`、`output`、`cache_read`、`cache_write`）、`model_multiplier`、`conditional_multipliers` 和 `branches`。单价单位为 **美元／百万 Token**。`PUT /api/v1/pricing/models` 是完整配置写入，以上字段必须齐全；空数组 `[]` 才表示清空规则或分支。明确填写的零价有效，与缺价不同。

事件先匹配实际模型，未命中时再匹配别名。命中分支后，四项分支单价替换**整条请求**的基础单价；没有命中则使用默认基础价。分支的上下文按归一化 `input_tokens` 判断（`all`、`gt`、`lte` 或 `range`）；每日时段可为 `all` 或 `HH:mm` 窗口，按已存 CPA 事件时间与部署时区匹配，含起点、不含终点，并支持跨午夜。互相重叠的分支不能保存。模型倍率和所有命中的条件倍率作用于整条请求。需要计价但缺少模型价格的事件以金额 0、`cost_available: false` 保存；明确设置的免费价格仍为可用费用。

`GET /api/v1/pricing/sync/fetch?source=models-dev`（或 `litellm`）只拉取待审核候选项，不保存配置。`POST /api/v1/pricing/sync/apply` 提交选中的 `model`、`pricing_style` 和四项 `base_prices`。对**已有模型**，服务端只更新四项基础价，保留已存价格风格、模型倍率、条件倍率和分支；**新模型**使用来源风格，倍率为 `1`，规则和分支为空。应用来源价格不重算已存事件。

## 重算历史费用

价格页面提供按所选近期热事件区间对**全部模型**手工重算；它不是逐模型任务。先读取 `GET /api/v1/pricing/recalculations/options`。服务端返回部署时区、合法绝对小时起点、3,600 秒步长、最多 30 天和当前 `config_revision`。将选中的 `start_at` 以带时区偏移的 ISO 8601 时刻，连同该修订号发送到 `POST /api/v1/pricing/recalculations`。受理时会再次核对边界与修订，并固定结束时刻。处理区间为 `[start_at, end_at)`；热窗口以外的归档事件不在目标中。

每个进程同时只有一个任务。运行中重复启动会返回现有任务和 `started: false`，不会排队。轮询 `GET /api/v1/pricing/recalculations/current` 可查看 `running`、`completed` 或 `failed` 及已提交行数。任务仅在内存中：进程重启后 `current` 返回 `null`，不会自动续跑。每一已提交页会在同一事务内更新事件、小时和日费用；失败或进程停止后，这些页仍保留。排除故障后，应对需要的范围**重新启动**一次重算，不能假定原任务继续。再次重算以新费用替换旧值并调整汇总差额，不会重复累加请求数或 Token。

重算期间，受影响的费用读取 API 返回 HTTP `503`、业务码 `costs_busy`；价格配置写入返回 HTTP `409`、业务码 `pricing_busy`。Keeper 仍可将新 CPA 消息持久接收到 inbox，普通事件处理和聚合暂缓，恢复后继续处理。选项和当前任务接口可继续用于查看进度。

## 现有数据库的首次升级

保留现有数据库及备份目录，并为完整 M1 备份、WAL 和迁移操作预留磁盘空间。旧库首次由新版启动时，Keeper 会在任何破坏性历史迁移前创建并核验一份受保护的 **M1 备份**。随后执行待补的已发布结构迁移，固定现有冷热事件与汇总水位，再按已提交的页回填事件费用和小时、日总费用；价格数据核验通过后才开放完整业务 API。全新空库走新库建表路径，不需要旧库 M1 备份。

迁移期间启动外壳仍可访问。用 `GET /api/v1/startup/status` 查看 `opening`、`migrating`、`failed` 或 `ready`；`/healthz` 返回正常并不表示业务 API 已就绪。出现 `failed` 时，应先排查再继续。不要让旧版 Keeper 二进制连接部分升级或已升级的新数据库。

### 首次升级失败后的恢复

离线工具 `pricing-restore` 只用于**带有效受保护 M1 备份的旧库首次升级失败**。它以备份生成新的旧结构数据库，再补回备份之后已进入故障库 inbox 的消息；不是通用降级工具，也不是普通定时备份恢复工具。

1. 停止 Keeper 和所有其他数据库写入者。保留故障 `app.db` **及其 `-wal`、`-shm` 附属文件**，原 M1 备份也留在记录的原路径。不要只复制故障主文件、删除附属文件或移动 M1 备份；工具会用故障库的保护状态核对备份身份。
2. 从本源码构建工具并选择**全新**输出目录，把示例备份路径替换为实际记录的 M1 文件名。输出 `app.db` 及其附属文件名都不得已存在：

   ```bash
   go build -o ./pricing-restore ./cmd/pricing-restore
   mkdir -p ./recovery
   ./pricing-restore \
     -backup "./old-work/backups/M1-backup.db" \
     -fault "./old-work/app.db" \
     -out "./recovery/app.db"
   ```

3. 核对工具打印的 M1 inbox 最大 ID、故障 inbox 最大 ID 和补回行数。工具检查 SQLite 文件完整性及记录的 M1 边界，按备份中的**实际最大 ID 之后**至故障库最大 ID 补回每一行，不按内容去重；原 ID、原文、哈希和接收时间都保留。补回的行，包括故障库里标记为已处理的行，会改为待处理，因为对应事件不在恢复出的备份中。备份和故障库都不会被覆盖。
4. 保持旧工作目录原样。把新生成的 `app.db` 放在独立 `WORK_DIR` 中（如显式配置了备份目录，也使用独立目录），用**当前版本** Keeper 启动。等待 `/api/v1/startup/status` 到达 `ready`，核对用量和 inbox 消费；验证完成前保留故障库、附属文件和 M1 备份。**不要**让旧二进制直接连接已升级库，也不要把恢复文件误认为已经升级完成；当前版本还需对其重新执行迁移。

若输出文件或附属文件名已经存在，或 M1 备份与故障库记录不符，工具拒绝发布结果。修正输入后改用新的输出路径，不要覆盖证据文件。

## 价格 API 兼容性

下表路径均位于 Keeper 的 `/api/v1` 下；若配置 `APP_BASE_PATH`，前面还需加部署子路径。价格接口要求管理员权限。

| 方法和路径 | 合同 |
| --- | --- |
| `GET /pricing/models` | 返回可编辑的完整模型配置及 `config_revision` |
| `GET /pricing/model-options` | 返回候选模型名 |
| `PUT /pricing/models` | 完整替换一个模型配置并返回新修订 |
| `DELETE /pricing/models?model=...` | 删除当前配置并返回新修订 |
| `GET /pricing/sync/fetch?source=...` | 从 `models-dev` 或 `litellm` 拉取候选基础价供审核 |
| `POST /pricing/sync/apply` | 应用 `source` 和选中 `items[]` 的 `model`、`pricing_style`、四项 `base_prices` |
| `GET /pricing/recalculations/options` | 获取服务端计算的起点和修订 |
| `POST /pricing/recalculations` | 使用 `start_at` 和 `config_revision` 启动 |
| `GET /pricing/recalculations/current` | 当前进程任务；无任务返回 `null` |

旧 `GET`／`PUT`／`DELETE /pricing`、`PUT /pricing/:model`、`GET`／`PUT /pricing/rules`、`PUT /pricing/batch`、`GET /pricing/sync/preview` 和 `GET /models/used` 已移除，返回 `404`。API 调用方需改用上述完整模型、同步与重算合同。

Analysis 响应将 `cost_breakdown` 替换为 `cost_summary`，只包含 `total_cost_usd` 和 `cost_available`。原来的 `uncached_input_cost_usd`、`cache_read_cost_usd`、`cache_write_cost_usd`、`output_cost_usd` 已删除；依赖旧对象或这些字段的客户端需要调整。Token 数仍在 Analysis 的常规数据字段中。

## 实测规模与适用边界

[价格规模测试报告](../internal/benchmark/pricing-results/2026-09-24/README.zh.md)记录了可复现的 100 万／1000 万冷热合成数据、独立旧五维迁移实验，以及仅在 100 万规模做的同事实集 119 天旧新 HTTP 对照。在所记录的 6 vCPU Linux 测试机上，M1–M6 价格迁移分别用时 **400.913 秒**与 **3,573.025 秒**；1000 万任务的 cgroup 内存峰值为 **6.85 GiB**，采样到的主库／WAL／备份合计峰值为 **12.927 GiB**。旧五维与 Latency 回放每页从 1,000 行缩到 100 行后，在约 50 条／秒的测试 inbox 负载下消除了丢弃，但五维场景 M1–M6 总时长增加，提供至提交仍最长等待 **26.001 秒**。这些是指定硬件、数据分布和规则数量下的观测，不是其它配置的最低资源需求或性能保证。
