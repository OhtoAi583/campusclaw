# Design: 认证授权与知识库入库（迭代 1）

## Context

一次请求只有一条链路：浏览器 → Nginx → Go → MySQL。浏览器与静态页一律不可信，
认证、授权、班级隔离与上传校验全部发生在 Go 进程内。数据库只在 Compose 内部网络可达，
上传目录只挂载到 api 容器，不经 Nginx 静态暴露。

会话状态存在服务端 `sessions` 表，进程内还保存登录失败计数；两者在多副本下会不一致，因此本迭代声明单实例。

## Decisions

### Decision 1: 同源 `/api` 反向代理（D1）

前端只调用同源 `/api`，开发时 Vite 的 proxy 与生产 Nginx 的 `location /api` 指向同一路径。
这样 Cookie 不跨站，`SameSite=Lax` 足够；Web 只需一个对外端口。

备选：前端直连后端端口。否决理由：跨域会引入 CORS 与 Cookie 的作用域问题，且需要额外对外暴露 api 端口，扩大攻击面。

### Decision 2: 服务端会话 Cookie 承载 user_id / role / class_id（D2）

`POST /api/login` 校验口令后写入 `sessions` 行，浏览器只拿到一个不透明随机 token（`HttpOnly` Cookie）。
授权与隔离需要 user_id、role、class_id，这三项每次请求都从 `sessions` → `users` 表读取，不采信客户端回传的任何身份字段。

备选：JWT 放在 localStorage。否决理由：脚本可读，出现 XSS 即被窃取；载荷里的 role / class_id 容易被当作授权依据；
无状态 token 在过期前难以作废，登出无法立即生效，仍需引入黑名单，等于绕回服务端会话。

### Decision 3: 班级只来自会话；跨班与不存在同形 404（D3）

列表查询强制 `WHERE class_id = <会话班级>`。按 ID 取详情或文件时先取行再核对班级（fetch-then-check），
不在 SQL 里提前过滤班级；这样"跨班"与"ID 不存在"对外返回完全相同的 404 响应，资源存在性不泄漏，
真实原因只写服务端日志。

备选：跨班返回 403。否决理由：403 等于承认资源存在，攻击者可以遍历 ID 枚举资源；本需求选 404 并全文一致，排障改走日志。

### Decision 4: 认证失败统一文案，并对用户名 + IP 限流（D4）

用户名不存在与口令错误返回同一个状态码与同一段文案，未知用户也执行一次口令哈希比较以拉平耗时。
锁定期的响应与凭据错误一致，不用 429 暴露"这个账号存在且已被锁定"。

备选：分别提示"用户不存在"与"密码错误"。否决理由：这会直接暴露账号是否存在，使枚举成本降到一次请求。

### Decision 5: 白名单扩展名 + 服务端生成存储名（D5）

只接受 `.txt` / `.md`，其余扩展名 400。存储名由服务端生成，客户端文件名只作为展示标题，不参与任何路径构造。

备选：黑名单排除可执行扩展名。否决理由：黑名单无法穷举危险类型（`.md.exe`、双扩展名、大小写变体）。

### Decision 6: 上传是一次事务，失败不留残留（D6）

校验（扩展名、大小、UTF-8 与非空）在写磁盘之前完成；随后写入上传目录，再在同一事务中写入
`materials` 与 `knowledge_entries`。任何一步失败都要回滚事务并删除已写入的文件，不允许出现孤儿文件或孤儿记录。

备选：先入库再异步写文件 / 只写磁盘不入库。否决理由：前者会让第 4 课的检索拿到没有正文的记录，
后者会让下载 404；两者都产生无法判定的半成品状态。

### Decision 7: 上传目录不静态暴露（D7）

原文件只落在 api 容器的挂载目录，读取必须经过 `GET /api/materials/{id}/file` 的鉴权与班级核对。
Nginx 只托管前端构建产物并反代 `/api`。

备选：把上传目录挂给 Nginx 当静态目录。否决理由：静态路径不受会话控制，任何人猜到文件名即可下载他人班级材料。

### Decision 8: 配置外置且缺项即停（D8）

`SESSION_SECRET`、数据库凭据、种子口令、上传上限与会话 TTL 全部从环境变量读取，没有内置默认密钥，
缺失时启动失败。`.env` 不入库也不进镜像，`.env.example` 列出全部必填项。
`SESSION_SECRET` 用于对会话 token 做 HMAC 后入库，因此它同时决定会话标识的签发与校验。

备选：内置开发默认密钥，克隆即可运行。否决理由：等于把会话标识的签发权交给任何拿到仓库的人。

### Decision 9: `/health` 只做存活判定（D9）

`GET /health` 无需登录，只回答进程是否存活，不与数据库探活混成同一语义；
数据库故障表现为受保护接口不可用（503），而不是把已登录用户判为会话失效（401），也不触发容器重启放大故障。

备选：`/health` 里直接查询数据库。否决理由：把存活与就绪混在一起，数据库抖动会导致容器被反复重启。

## Interface Overview

| 方法 | 路径 | 鉴权 | 成功 | 失败 |
| --- | --- | --- | --- | --- |
| GET | `/health` | 公开 | 200 | — |
| POST | `/api/login` | 公开 | 200 + Set-Cookie | 401（统一文案，含锁定期） |
| POST | `/api/logout` | 会话 | 204 | 401 |
| GET | `/api/me` | 会话 | 200 | 401 |
| GET | `/api/materials` | 会话 | 200（仅本班） | 401 |
| POST | `/api/materials` | 教师 | 201 | 401 / 403 / 400 / 413 |
| GET | `/api/materials/{id}` | 会话 | 200（本班） | 401 / 404（跨班与不存在同形） |
| GET | `/api/materials/{id}/file` | 会话 | 200（附件） | 401 / 404（跨班与不存在同形） |

## Data Model

- `classes(id, name)`：班级边界，(name) 唯一。
- `users(id, username, password_hash, role, class_id, created_at)`：`role ∈ {teacher, student}`，`username` 唯一，`class_id` 非空。
- `sessions(id, user_id, created_at, expires_at)`：`id` 为 HMAC 后的会话标识，登出即删行。
- `materials(id, class_id, title, original_name, stored_name, size_bytes, uploaded_by, created_at)`：`class_id` 非空并建索引。
- `knowledge_entries(id, material_id, class_id, content, created_at)`：本迭代只存正文，`class_id` 非空并建索引，不提前向量化。

## Configuration & Startup

标准启动方式只有一种：`cp .env.example .env` → 填必填项 → `docker compose up --build -d` → `http://localhost:8080`。
api 启动时重试等待数据库可连接（`depends_on` 只保证启动顺序），随后执行建表与幂等种子。
数据库使用普通账号连接，管理员口令只用于 db 容器初始化，不下发给 api。
