# Change: 认证授权与知识库入库（迭代 1）

## Why

CampusClaw 的后续能力（备课、命题、学情分析）都建立在一份可按班级隔离的教研材料与知识库正文之上。
当前仓库没有任何身份、权限与数据边界约定：谁都能看到任何材料，材料也没有可检索的正文。
先把这个底座做出来并让它可被第三方按 README 复现，后续迭代才有可继承的基线。

本次变更同时是第 2 课 Propose 产物的实施入口：它把"做到什么算合格"写成可判定的 Requirement 与 Scenario，
再由 tasks.md 逐条驱动实现、校验与归档。

## What Changes

- 新增账号密码登录与服务端会话：登录成功换发新会话 ID，登出删除服务端会话行，Cookie 为 `HttpOnly` + `SameSite=Lax`。
- 新增角色授权：教师可上传、查看、下载本班材料；学生只能查看与下载。
- 新增班级数据边界：租户标识只取自服务端会话，列表按班级过滤，按 ID 读取先取行再核对班级。
- 新增材料上传与知识库入库：教师上传 `.txt` / `.md`，同一事务写入 `materials` 与 `knowledge_entries`，失败不留残留。
- 新增最小 Web（登录页 + 材料页）与同源 `/api` 反向代理，Docker Compose 单实例交付。
- 新增种子数据：A / B 两个班级、三个预置账号、两班标题可区分的材料。
- 新增 README、`.env.example`、`.dockerignore`、迭代说明与 ADR。

## Impact

- 新增能力：`auth-upload`
- 受影响代码：`backend/`（Go + net/http）、`frontend/`（React + TypeScript + Vite）、`deploy/`（Nginx）、`docker-compose.yml`
- 受影响数据表：`classes`、`users`、`sessions`、`materials`、`knowledge_entries`
- 配置：`SESSION_SECRET`、数据库凭据、上传目录与上限、会话 TTL、登录失败阈值、种子口令均来自环境变量；
  `.env` 不入库，`.env.example` 入库；缺必填项时启动失败。

## Non-goals（非目标）

以下条目本迭代明确不做。缺失的"不做"会让实现越界到未评审的范围，因此逐条写明：

- 知识库问答、向量检索、RAG：本迭代的知识库只负责保存可查询的正文。
- 对话助手、作业布置与批改、成绩与错题本。
- JWT / OAuth / SSO：登录态使用服务端会话 Cookie，不使用 localStorage 中的 Bearer token。
- 平台超级管理员角色：跨班可见会与"跨班返回 404"的隔离验收直接冲突，隔离稳定后另开变更设计。
- 注册、改密、多校多租户、验证码、邮箱或短信登录。
- PDF / Word / 图片解析，以及在线预览与编辑；本迭代只解析纯文本类材料（`.txt` / `.md`）。
- Kubernetes、CI、公网域名、HTTPS 证书、多副本高可用；本迭代为单实例 Compose。
- 前端全文检索：材料页的搜索只在本班已加载的数据范围内过滤标题与正文。
