# 公共档案去标识审核服务

本项目是面向公共档案开放审核员和独立复核员的 Go HTTP 服务。它把一宗待开放档案依次推进为草稿、待去标识、待复核、退回修改、已批准和已发布状态，并用案件 `revision` 防止并发覆盖，用请求标识实现写操作幂等。服务使用可解释规则检测身份信息、联系方式和受限编号，提供不含匹配原文的风险摘要、按职责隔离的工作队列、原子批量处置、独立复核预检和定向退回。批准后冻结材料和审核结论，发布前可核对确定性的去标识内容、内容指纹及发布清单，并可只读校验整宗案件的审计证据完整性。

## 构建与测试

项目要求 Go 1.22 或更高版本。

```bash
go build ./cmd/server
go test ./...
```

端到端自检会创建隔离的临时数据目录，启动真实 HTTP 服务，完整执行检测、首次处理、退回、重新处理、批准和发布流程，然后自行关闭：

```bash
go run ./cmd/server -selftest -addr=127.0.0.1:19081
```

## 运行服务

默认只监听高位回环地址 `127.0.0.1:19081`：

```bash
go run ./cmd/server
```

可显式指定回环地址和持久化目录：

```bash
go run ./cmd/server -addr=127.0.0.1:19181 -data-dir=./data
```

也可以设置仅含端口号的 `PORT`，服务会推导为 `127.0.0.1:<PORT>`。显式地址必须使用可解析的回环 IP；服务拒绝 `0.0.0.0`、主机名和非回环地址。`PORT` 未设置时不会使用 `8080`、`80`、`3000` 等常见端口作为默认值。

正常模式响应 `SIGINT` 和 `SIGTERM`，并在十秒超时内优雅关闭。

## API 流程

所有接口返回 JSON。写请求必须使用 `Content-Type: application/json`，提供 `X-Actor-ID` 和全局唯一的 `X-Request-ID`；除创建案件外还须通过 `If-Match` 提供当前案件 `revision`。响应的 `ETag` 也包含 revision。相同 `X-Request-ID` 对同一操作的重放返回首次成功写入时保存的结果；把同一标识用于其他操作会返回 `409 Conflict`。

主要路由如下：

| 方法 | 路径 | 用途 |
| --- | --- | --- |
| `GET` | `/healthz` | 健康检查 |
| `POST` | `/api/v1/cases` | 创建草稿案件 |
| `GET` | `/api/v1/cases/work-queue` | 按操作者职责查询工作队列、状态统计和游标分页 |
| `GET` | `/api/v1/cases/{caseID}` | 查询案件及发现项、复核记录 |
| `POST` | `/api/v1/cases/{caseID}/detect` | 执行敏感片段检测 |
| `PATCH` | `/api/v1/cases/{caseID}/findings/{findingID}` | 接受、调整或驳回单个发现项 |
| `POST` | `/api/v1/cases/{caseID}/findings/batch-decisions` | 原子提交最多 100 项发现项决定 |
| `GET` | `/api/v1/cases/{caseID}/risk-summary` | 查询分类、覆盖、风险依据和决定状态摘要 |
| `GET` | `/api/v1/cases/{caseID}/review-submissions/readiness` | 以 `reviewer_id` 预检独立复核就绪状态 |
| `POST` | `/api/v1/cases/{caseID}/review-submissions` | 指定独立复核人并提交复核 |
| `POST` | `/api/v1/cases/{caseID}/review-decisions` | 退回或批准案件 |
| `GET` | `/api/v1/cases/{caseID}/manifest/preview` | 授权人员预览候选发布清单证据 |
| `POST` | `/api/v1/cases/{caseID}/publish` | 发布已批准案件 |
| `GET` | `/api/v1/cases/{caseID}/manifest` | 查询已发布清单 |
| `GET` | `/api/v1/cases/{caseID}/timeline` | 查询状态时间线 |
| `GET` | `/api/v1/cases/{caseID}/audit-events` | 查询完整审计事件 |
| `GET` | `/api/v1/cases/{caseID}/audit-events/integrity` | 只读校验快照、修订、事件和发布证据 |

检测结果中的 `start_offset` 和 `end_offset` 是原始 UTF-8 内容的字节偏移。`accept` 默认替换为 `[已遮蔽]`，`adjust` 必须提供非空 `replacement`，`reject` 表示确认规则误报并保留原文；三种决定都必须填写 `reason`。

创建案件时会按规范化内容的 SHA-256 摘要检索尚在系统内的相同材料。存在重复且请求未设置 `allow_duplicate` 时返回 `409 Conflict`，响应 `details` 给出关联案件标识、状态、来源部门及同部门或跨部门分类，不产生案件、审计或幂等写入。继续受理须同时设置 `allow_duplicate: true` 和非空 `duplicate_reason`，理由只以摘要形式进入重复证据和审计载荷。

工作队列要求 `X-Actor-ID`，支持 `status`、`source_department`、RFC3339 格式的 `updated_from` 与 `updated_to`、`page_size` 和 `cursor` 查询参数。提交人只看到自己负责的待去标识或退回修改案件，复核人只看到指派给自己的待复核案件；摘要不含材料原文和敏感匹配文本。

批量处置请求使用 `decisions` 数组，每项包含 `finding_id`、`decision`、`reason` 和按决定需要的 `replacement`。任一项失败会拒绝整个批次；成功批次仅递增一次 revision 并产生一个审计事件。复核退回可通过 `remediation_items` 指定 `finding_id` 和非空 `instruction`，省略该数组时兼容原有的全部重置行为。就绪预检返回的 `preflight_revision` 可随正式提交复核发送；发布预览返回的 `preview_revision` 和 `content_fingerprint` 可随发布请求发送，任一证据变化都会返回 `409 Conflict`。

## 持久化

默认数据目录为 `./data`。`cases/` 保存版本化案件 JSON 快照，快照先写入同目录临时文件并执行同步，再通过原子替换提交；`audit.jsonl` 是只追加的审计事件日志；`requests.json` 保存幂等键及当次结果；`content-digests.json` 保存创建时同步维护的内容摘要索引。服务读取和重启时校验快照数据版本、源内容摘要、内容摘要索引、风险摘要 revision、审计序列和幂等结果，损坏数据不会被静默接受。
