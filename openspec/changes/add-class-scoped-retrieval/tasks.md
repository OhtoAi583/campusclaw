# Tasks: 限定班级范围的知识库检索与内容溯源（迭代 2）

## Apply 约定

- 一条 task 一次循环：实现 → 对照 spec 审查 diff → 运行该行 verify → 通过后勾选并提交。
- 失败的 task 不带入下一项。
- Apply 期间只允许改本 change 的 delta（`openspec/changes/add-class-scoped-retrieval/`），不改 `openspec/specs/`。
- verify 栏是可执行判据；没跑过的项不得勾选。
- 开工前确认：`openspec list` 只有本 change；`openspec/specs/` 里还没有 `kb-retrieval`。

## 1. 数据模型与配置

- [ ] 1.1 新增 `kb_chunks` 表：`material_id`、`class_id`（非空 + 索引）、`chunk_index`、`start_offset`、`end_offset`、`content`、`embedding`、`embedding_dim`，并建 `(material_id, chunk_index)` 唯一键 — verify: 启动后 `show create table kb_chunks` 可见非空约束、索引与唯一键
- [ ] 1.2 新增配置项 `EMBEDDING_DIM`、`CHUNK_SIZE`、`CHUNK_OVERLAP`、`SEARCH_TOP_K_MAX`、`SEARCH_QUERY_MAX_CHARS`、`SEARCH_TIMEOUT_MS`，一并写入 `.env.example` — verify: `grep -c` 变量数与 config 读取项一致
- [ ] 1.3 配置非法时启动失败：`CHUNK_OVERLAP >= CHUNK_SIZE`、`EMBEDDING_DIM <= 0`、`SEARCH_TOP_K_MAX <= 0` — verify: 分别用非法值启动，进程非零退出并打印非法项
- [ ] 1.4 `EMBEDDING_DIM` 与库中 `embedding_dim` 不一致时启动失败并提示重建 — verify: 改 `EMBEDDING_DIM` 后启动，观察退出信息

## 2. 分块与向量化

- [ ] 2.1 实现分块：优先按 Markdown 标题切分，超长段落按 `CHUNK_SIZE` + `CHUNK_OVERLAP` 切分，记录字符区间 — verify: 单测断言块的并集覆盖正文、块内容等于正文子串
- [ ] 2.2 实现 `Embedder` 接口与默认本地确定性嵌入（字符 n-gram + 哈希技巧 + L2 归一化）— verify: 单测断言同输入同输出、维度等于配置值
- [ ] 2.3 向量编解码：`float32` 小端定长 BLOB，长度 = `EMBEDDING_DIM × 4` — verify: 单测往返编解码一致；查库确认 `octet_length(embedding) = dim*4`
- [ ] 2.4 实现余弦相似度排序（向量已归一化时退化为点积），同分时用 `material_id`、`chunk_index` 稳定排序 — verify: 单测断言排序可复现

## 3. 索引写入、同步与重建

- [ ] 3.1 上传材料时在同一事务写入 `materials` + `knowledge_entries` + `kb_chunks`，失败整体回滚并删除已写入文件 — verify: 制造一次索引写入失败，检查三表无记录且上传目录无新增文件
- [ ] 3.2 提供幂等重建：先删该材料的旧块再重建，重复执行块数不变 — verify: 连续执行两次，比对 `kb_chunks` 行数与区间
- [ ] 3.3 启动时为缺块的历史材料（迭代 1 的种子与既有上传）补建索引 — verify: 清空 `kb_chunks` 后重启，缺块材料被补建
- [ ] 3.4 重建命令可单独执行 — verify: 运行重建命令，输出处理材料数与块数

## 4. 检索接口与班级隔离

- [ ] 4.1 实现 `POST /api/search`：解析 `query` 与 `top_k`，走鉴权中间件 — verify: 已登录返回 200，未登录返回 401
- [ ] 4.2 候选集在 SQL 层按会话班级过滤，应用层二次断言 — verify: 阅读查询语句确认带 `class_id = ?`，且断言分支存在
- [ ] 4.3 请求体或查询串里的 `class_id` 被忽略 — verify: 附带 `class_id=B` 检索，结果仍只含 A 班材料
- [ ] 4.4 跨班关键词检索返回空数组，且响应不含 B 班任何标识 — verify: 用 B 班独有词检索，`items` 为空且响应体无 B 班标题
- [ ] 4.5 参数校验：空查询 / 超长查询 / `top_k` 越界均返回 400 — verify: 三类请求各返回 400
- [ ] 4.6 无命中返回 200 + 空数组（不是 404） — verify: 用一个不存在的词检索
- [ ] 4.7 超时返回 503，不返回部分结果 — verify: 把 `SEARCH_TIMEOUT_MS` 调至极小值后检索，观察 503

## 5. 内容溯源

- [ ] 5.1 结果字段包含 `material_id`、`material_title`、`original_name`、`chunk_index`、`start_offset`、`end_offset`、`content`、`score` — verify: 对照 spec R3.1 逐字段核对响应
- [ ] 5.2 片段与正文区间一致：按 `start_offset`/`end_offset` 截取详情正文与 `content` 逐字符相同 — verify: 写脚本对每一条结果断言一致
- [ ] 5.3 响应中不出现存储名与磁盘路径；同班可读详情、跨班仍 404 — verify: 检查字段 + 用两个班级会话各读一次 `material_id`
- [ ] 5.4 响应中的 `material_id` 一定是本班材料 — verify: 遍历结果，逐条查库核对 `class_id`

## 6. 前端检索页与跳转

- [ ] 6.1 新增检索页：输入查询、调用 `/api/search`、展示片段与来源标题 — verify: 操作界面能看到片段与标题
- [ ] 6.2 结果按相关度排序展示，并显示分数或相关度提示 — verify: 界面检查
- [ ] 6.3 点击结果跳转到材料详情并定位/高亮对应片段 — verify: 从检索结果点进详情，目标片段可见高亮
- [ ] 6.4 无结果时显示明确提示，不展示其他班级内容 — verify: 用 B 班关键词搜索，界面显示"没有找到相关内容"
- [ ] 6.5 检索请求 401 时清空状态回登录页 — verify: 删除 Cookie 后触发检索，界面回到登录页

## 7. 文档与配置说明

- [ ] 7.1 README 增补检索章节：接口、请求 / 响应示例、检索范围、已知限制（词形相似而非语义泛化） — verify: 只读 README 能回答"检索范围是什么、结果能否溯源"
- [ ] 7.2 README 写明重建索引用法 — verify: 按 README 执行重建命令成功
- [ ] 7.3 `.env.example` 补齐检索配置并说明各参数含义 — verify: 逐项对照 config 代码

## 8. 评测与验收

- [ ] 8.1 建立固定评测集（查询 + 期望来源 + 隔离用例） — verify: 评测文件存在且格式可被脚本读取
- [ ] 8.2 评测脚本输出命中率与逐条结果 — verify: 运行脚本，输出命中率不低于阈值
- [ ] 8.3 隔离用例全部通过（B 班关键词在 A 班检索为空） — verify: 评测输出中隔离用例全绿
- [ ] 8.4 关键 Scenario 逐条给出判定结论与证据 — verify: 证据文档中对 R1.3 / R3.2 / R6.1 / R6.2 等给出命令与输出
- [ ] 8.5 记录失败路径证据：未登录 401、跨班空结果、参数 400、超时 503 — verify: 四条命令与输出留档

## 9. 发布与归档

- [ ] 9.1 `openspec validate add-class-scoped-retrieval --strict` 通过 — verify: 退出码 0 且无 error
- [ ] 9.2 按 README 从零启动后检索可用（含种子数据已建索引） — verify: 全新 `down -v` 后 `up --build -d`，登录后检索命中种子材料
- [ ] 9.3 打版本 tag（建议 `v0.2.0-retrieval`） — verify: `git tag` 列出该 tag
- [ ] 9.4 归档：活动变更清零、`changes/archive/` 出现带日期目录、`openspec/specs/kb-retrieval/spec.md` 生成 — verify: `openspec list` 无本 change，`openspec list --specs` 出现 `kb-retrieval`
