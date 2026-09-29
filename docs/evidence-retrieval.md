# 迭代 2 验收证据：班级范围检索与内容溯源

所有命令在本机 `http://127.0.0.1:8088`（Nginx → Go → MySQL）实测，数据为种子材料（A 班语文、B 班数学各一篇）。

## 一、检索命中与内容溯源

```
$ curl -s -b cookie.jar -X POST $WEB/api/search -H 'Content-Type: application/json' \
    -d '{"query":"导入环节的设计意图","top_k":3}'

class_id = 1  count = 2
  material=23 chunk=1 [320-588] score=0.324  A班·教研材料示例：语文阅读课教学设计
    片段: ## 导入环节的设计意图  导入环节用一张生活情境图引发预测……
  material=23 chunk=0 [0-320]   score=0.169  A班·教研材料示例：语文阅读课教学设计
```

**溯源一致性**：按结果的 `start_offset`/`end_offset` 去该材料详情正文上截取，与结果里的 `content` 比对：

```
$ 对每条结果断言 content[start:end] == item.content
  material=23 chunk=1 -> 逐字符一致 ✔
  material=23 chunk=0 -> 逐字符一致 ✔
结论: 全部一致 ✔
```

## 二、班级隔离

```
$ A 班学生检索「分数四则运算 验算习惯」（只出现在 B 班材料里）
count = 0  items = []
```

响应体里没有 B 班标题、正文片段或 `material_id`。请求里带 `class_id=2` 时返回的 `class_id` 仍为 1。

## 三、逐条对照规约的验收

```
$ ./scripts/verify-retrieval.sh
R1 检索范围只限于会话所在班级
  ✔ 未登录检索返回 401（401）
  ✔ A 班检索 B 班关键词返回空数组（0）
  ✔ 请求带 class_id=B 被忽略（仍返回 A 班）（1）
  ✔ 跨班检索响应不含对方标题（0）
R2 检索接口与参数校验
  ✔ 正常检索返回 200（200）
  ✔ 空查询返回 400（400）
  ✔ 超长查询返回 400（400）
  ✔ top_k 超上限返回 400（400）
  ✔ 无命中返回 200 + 空数组（200 0）
R3 内容溯源
  ✔ 结果字段完整
  ✔ 结果不含存储名与磁盘路径
  ✔ 片段与正文区间逐字符一致
R5 向量与排序
  ✔ 向量长度 = 维度 × 4（2048）
  ✔ 同样查询两次排序一致（[(23, 1), (23, 0)]）
R6 索引同步
  ✔ 教师上传返回 201（201）
  ✔ 上传后立即可检索到（1）
全部通过 ✔
```

## 四、质量门禁（评测集）

```
$ ./scripts/eval-retrieval.sh
  ✔ A班·导入环节：命中 A班·教研材料示例：语文阅读课教学设计（score=0.324）
  ✔ A班·分层作业：命中 A班·教研材料示例：语文阅读课教学设计（score=0.256）
  ✔ A班·教学反思：命中 A班·教研材料示例：语文阅读课教学设计（score=0.221）
  ✔ B班·验算习惯：命中 B班·教研材料示例：数学课堂练习设计（score=0.374）
  ✔ B班·常见错误：命中 B班·教研材料示例：数学课堂练习设计（score=0.377）
  ✔ 隔离·A班检索B班关键词：未返回跨班内容（结果数 0）
  ✔ 隔离·B班检索A班关键词：未返回跨班内容（结果数 0）
命中率：5/5 = 100%（阈值 80%）
隔离用例：全部通过 ✔
```

## 五、失败路径

| 用例 | 命令要点 | 结果 |
| --- | --- | --- |
| 未登录检索 | 不带 Cookie `POST /api/search` | **401** `{"error":"unauthorized"}` |
| 空 / 超长查询 | `{"query":"   "}`、300 字查询 | **400** `invalid_query` |
| `top_k` 越界 | `{"query":"阅读","top_k":999}` | **400** |
| 无命中 | 不存在的词 | **200** `items: []`（不是 404） |
| 检索超时 | 按住 `kb_chunks` 写锁让查询阻塞 | **503**，耗时 1.55s（超时配置 1.5s），无部分结果 |
| 索引写入失败 | `rename table kb_chunks` 后上传 | **503**，`materials` 行数不变、上传目录无新增文件 |

## 六、幂等重建与历史数据补建

```
$ ./bin/reindex
索引重建完成：材料 2 篇，块 2 个，向量维度 512
$ ./bin/reindex      # 再执行一次
重建前块数=2  重建后块数=2   → 幂等 ✔

$ 启动日志：{"msg":"已为历史材料补建索引","materials":3}
```

## 七、脚本本身的两个坑（记录一下）

验收脚本在开发过程中也修了两个问题，都是脚本缺陷而非应用缺陷：

1. **写死材料 id**：迭代 1 的 `verify.sh` 原本固定用 `id=1` 下载，重新播种后 id 变成 23/24 就 404 了。
   改为从 `/api/materials` 动态发现本班与跨班各一条材料 id。
2. **shell 里手写 JSON 转义**：`verify-retrieval.sh` 的请求体在多层引号里被写坏（`{}` 被吃掉），
   导致请求体非法。改为统一用 `python3 -c 'json.dumps(...)'` 生成请求体。

## 八、单元测试

```
$ (cd backend && go test ./...)
ok  campusclaw/backend/internal/kb        # 分块覆盖、分块稳定、嵌入确定性、向量编解码
ok  campusclaw/backend/internal/auth
ok  campusclaw/backend/internal/config
ok  campusclaw/backend/internal/materials
```

其中 `TestSplitKeepsContentWithDenseHeadings` 是本次开发中发现的真实缺陷的回归测试：
最初"合并过短段落"的逻辑会**直接跳过**两个标题之间的正文，导致长文档只索引到标题；
修复为"延长上一段的结束位置"后，块区间覆盖全文。
