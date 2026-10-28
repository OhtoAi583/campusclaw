# kb-retrieval Delta

## ADDED Requirements

### Requirement: R1 检索范围只限于会话所在班级

检索 MUST 在 `class_id = <会话班级>` 的候选集合内进行；租户标识 MUST 只来自服务端会话，
请求中的任何班级字段 MUST 被忽略，跨班内容 MUST NOT 出现在结果、计数或错误信息中。

#### Scenario: R1.1 只返回本班结果

- **WHEN** 持 A 班会话的客户端 `POST /api/search` 检索一个 A、B 两班正文中都出现过的词
- **THEN** 响应 200，且每条结果的来源材料都满足 `class_id = A`
- **AND** 结果中不含任何 `material_id` 属于 B 班的条目

#### Scenario: R1.2 请求参数不能扩大检索范围

- **WHEN** 持 A 班会话的客户端在请求体中附带 `class_id=B` 或查询串上附带 `?class_id=B`
- **THEN** 返回结果仍只来自 A 班

#### Scenario: R1.3 跨班关键词必须检索不到

- **WHEN** 持 A 班会话的客户端用只出现在 B 班材料中的关键词检索
- **THEN** 响应 200 且 `items` 为空数组
- **AND** 响应中 MUST NOT 出现 B 班材料的标题、正文片段或 `material_id`

#### Scenario: R1.4 未登录不可检索

- **WHEN** 未携带有效会话 Cookie 的客户端调用 `POST /api/search`
- **THEN** 响应 401，且响应体不含任何片段正文或材料标题

### Requirement: R2 检索接口与参数校验

系统 MUST 提供 `POST /api/search` 接受 `query` 与可选 `top_k`，并对参数做明确校验。

#### Scenario: R2.1 正常检索

- **WHEN** 持有效会话的客户端提交 `{"query":"<本班材料中出现过的词>","top_k":5}`
- **THEN** 响应 200，响应体包含 `query`、`class_id`、`count` 与 `items`
- **AND** `items` 的条数不超过 `top_k` 且不超过 `SEARCH_TOP_K_MAX`

#### Scenario: R2.2 空查询或超长查询被拒

- **WHEN** 提交的 `query` 为空、只有空白字符，或长度超过 `SEARCH_QUERY_MAX_CHARS`
- **THEN** 响应 400
- **AND** 不发生任何向量计算

#### Scenario: R2.3 top_k 超上限被拒

- **WHEN** 提交的 `top_k` 大于 `SEARCH_TOP_K_MAX` 或小于 1
- **THEN** 响应 400

#### Scenario: R2.4 无命中不是错误

- **WHEN** 查询合法但本班候选集内没有超过阈值的匹配
- **THEN** 响应 200 且 `items` 为空数组（MUST NOT 返回 404）

### Requirement: R3 内容溯源

每条检索结果 MUST 能指向具体材料与具体片段，且片段原文 MUST 与材料正文中对应区间逐字符一致。

#### Scenario: R3.1 结果字段完整

- **WHEN** 检索返回至少一条结果
- **THEN** 每条结果包含 `material_id`、`material_title`、`original_name`、`chunk_index`、`start_offset`、`end_offset`、`content`、`score`

#### Scenario: R3.2 片段与正文区间一致

- **WHEN** 取结果中的 `material_id` 调 `GET /api/materials/{id}` 拿到正文
- **THEN** 用 `start_offset` 与 `end_offset` 在该正文上截取出的子串与结果中的 `content` 逐字符相同

#### Scenario: R3.3 溯源仍受班级隔离约束

- **WHEN** 结果中的 `material_id` 由本班会话通过详情接口读取
- **THEN** 返回 200（同班可见）
- **AND** 若用其他班级的会话读取同一 `material_id`，仍按迭代 1 的规则返回 404

#### Scenario: R3.4 溯源信息不包含内部路径

- **WHEN** 检查任一结果的字段
- **THEN** MUST NOT 出现服务端存储名（`stored_name`）、上传目录或任何磁盘路径
- **AND** 文件下载仍必须走 `GET /api/materials/{id}/file` 鉴权接口

### Requirement: R4 正文分块

`knowledge_entries.content` MUST 被切成带字符区间的块，块 MUST 覆盖正文且不丢失内容。

#### Scenario: R4.1 分块覆盖正文

- **WHEN** 对一篇材料建立索引
- **THEN** 该材料所有块的 `[start_offset, end_offset]` 区间并集覆盖正文全部字符
- **AND** 任何块的 `content` 都等于正文对应区间的子串

#### Scenario: R4.2 块内序号与配置生效

- **WHEN** 查看某材料的块
- **THEN** `chunk_index` 从 0 开始连续递增
- **AND** 单块长度不超过 `CHUNK_SIZE`（除按标题切分产生的特例，必须在规约允许范围内）
- **AND** 相邻块的重叠字符数为 `CHUNK_OVERLAP`

#### Scenario: R4.3 同一正文分块稳定

- **WHEN** 对同一篇正文用相同配置重复建索引
- **THEN** 产生相同的块数量与相同的字符区间

### Requirement: R5 向量化与相似度检索

每个块 MUST 有一个与 `EMBEDDING_DIM` 等长的向量，检索 MUST 用余弦相似度排序，且同样的输入 MUST 得到同样的排序。

#### Scenario: R5.1 向量维度与存储一致

- **WHEN** 查询 `kb_chunks` 中任一行的 `embedding` 长度
- **THEN** 长度等于 `EMBEDDING_DIM × 4` 字节，且该行 `embedding_dim` 等于当前配置值

#### Scenario: R5.2 排序可复现

- **WHEN** 用同一查询重复检索两次（数据未变化）
- **THEN** 两次返回的 `material_id` 与 `chunk_index` 顺序完全一致

#### Scenario: R5.3 相关片段排在前

- **WHEN** 用某材料中出现过的独特词检索
- **THEN** 该材料对应的块出现在结果列表的首位

#### Scenario: R5.4 维度不一致时拒绝启动

- **WHEN** `EMBEDDING_DIM` 与库中已有 `kb_chunks.embedding_dim` 不一致时启动服务
- **THEN** 进程以非零状态退出，并提示需要重建索引
- **AND** MUST NOT 用旧向量继续提供检索

### Requirement: R6 索引同步与幂等重建

材料入库 MUST 同步建立索引；MUST 提供幂等的重建能力。

#### Scenario: R6.1 上传后立即可检索

- **WHEN** 教师上传一篇包含独特词 `X` 的 `.md`，随后立即用 `X` 检索
- **THEN** 检索结果包含该材料的溯源条目

#### Scenario: R6.2 重建幂等

- **WHEN** 对同一份数据连续执行两次重建
- **THEN** `kb_chunks` 中该材料的块数量在第二次执行后不增加
- **AND** 重建前后用同一查询得到的排序一致

#### Scenario: R6.3 索引失败与材料入库同进退

- **WHEN** 建立索引的过程中数据库写入失败
- **THEN** `materials`、`knowledge_entries`、`kb_chunks` 均无该次上传的记录
- **AND** 上传目录不留下已写入的文件

#### Scenario: R6.4 历史数据可补建

- **WHEN** 对迭代 1 时期已入库、尚无块的材料执行重建
- **THEN** 这些材料随后可被检索到

### Requirement: R7 检索页与结果跳转

前端 MUST 提供本班检索入口，结果 MUST 展示片段与来源，并支持跳转到材料详情。

#### Scenario: R7.1 检索入口与结果展示

- **WHEN** 已登录用户在检索页提交查询
- **THEN** 页面调用 `POST /api/search`
- **AND** 每条结果展示片段原文与来源材料标题

#### Scenario: R7.2 跳转并定位

- **WHEN** 点击一条检索结果
- **THEN** 打开该材料的详情页并定位到对应片段（可见高亮或滚动到该区间）

#### Scenario: R7.3 无结果提示

- **WHEN** 检索返回空数组
- **THEN** 页面显示"没有找到相关内容"，MUST NOT 展示其他班级的内容

#### Scenario: R7.4 401 回到登录页

- **WHEN** 检索请求返回 401
- **THEN** 前端清空状态并回到登录页

### Requirement: R8 检索质量门禁（评测集）

项目 MUST 提供固定评测集，用于判定检索质量与班级隔离。

#### Scenario: R8.1 评测集可执行

- **WHEN** 在验收环境运行评测（命令或脚本）
- **THEN** 输出每条用例的命中情况与总体命中率
- **AND** 命中率不低于评测集规定的阈值

#### Scenario: R8.2 隔离用例必须全过

- **WHEN** 运行评测集中标记为"隔离"的用例（用 B 班关键词在 A 班会话检索）
- **THEN** 全部返回空结果
- **AND** 任一用例出现跨班结果即判定本次交付不通过

### Requirement: R9 检索日志与审计

系统 MUST 记录检索请求的必要信息，且 MUST NOT 把查询原文与片段正文写入日志。

#### Scenario: R9.1 记录必要字段

- **WHEN** 完成一次检索
- **THEN** 日志包含 `user_id`、`class_id`、查询长度、候选数、结果数、耗时

#### Scenario: R9.2 不记录敏感内容

- **WHEN** 检查检索相关日志
- **THEN** MUST NOT 出现查询原文或片段正文

### Requirement: R10 配置与超时

检索相关参数 MUST 来自环境变量；超出超时的检索 MUST 以 503 结束，而不是返回不完整结果。

#### Scenario: R10.1 配置外置

- **WHEN** 阅读 `.env.example`
- **THEN** 列出 `EMBEDDING_DIM`、`CHUNK_SIZE`、`CHUNK_OVERLAP`、`SEARCH_TOP_K_MAX`、`SEARCH_QUERY_MAX_CHARS`、`SEARCH_TIMEOUT_MS`
- **AND** 仓库中不含真实密钥

#### Scenario: R10.2 超时返回 503

- **WHEN** 一次检索超过 `SEARCH_TIMEOUT_MS`
- **THEN** 响应 503
- **AND** 不返回部分结果

#### Scenario: R10.3 配置非法时拒绝启动

- **WHEN** `CHUNK_OVERLAP >= CHUNK_SIZE`，或 `EMBEDDING_DIM` 不是正整数
- **THEN** 进程以非零状态退出并指出非法配置项
