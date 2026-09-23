# CampusClaw · 迭代 1：认证授权与知识库入库

面向中小学教研场景的智能体底座。本迭代只交付**教研材料与知识库底座**：
登录与角色权限、班级数据边界、材料上传与知识库入库。

> 判断标准不是界面好不好看，而是：**能登录、能隔离、能上传入库、第三方能按 README 复现。**

## 范围（本迭代做什么）

- 账号密码登录 + 服务端会话（`HttpOnly` + `SameSite=Lax`），登录换发会话 ID 防会话固定，登出立即失效。
- 角色授权：**教师**可上传 / 查看 / 下载本班材料；**学生**只能查看 / 下载。
- 班级数据边界：租户标识只来自服务端会话；列表按班过滤，按 ID 读取「先取行再核对班级」。
- 材料上传（`.txt` / `.md`）：白名单校验、大小上限、UTF-8 校验、服务端生成存储名，
  `materials` 与 `knowledge_entries` **同事务**写入，失败不留残留。
- 最小 Web：登录页 + 材料页（本班搜索、列表 / 网格、深浅色、命令面板、上传进度、toast、Markdown GFM）。

## 不做（Non-goals）

向量检索、RAG、知识库问答、对话助手、作业批改、JWT / OAuth / SSO、平台超级管理员、
注册改密、多校多租户、PDF / Word / 图片解析、Kubernetes / CI / 公网域名 / HTTPS / 多副本。

## 关键约定（请先读这三条）

1. **跨班访问返回 404**，与"记录不存在"完全同形，不泄露资源是否存在；真实原因记录在服务端日志。
2. **未登录访问受保护接口返回 401**，响应体不含材料标题、正文或磁盘路径。
3. **单实例部署**：会话表在数据库，但登录失败限流在进程内存中，多副本下状态会不一致，本迭代不适用多副本。

## 技术栈

| 层 | 选型 |
| --- | --- |
| 前端 | React 18 + TypeScript + Vite（同源 `/api`） |
| 后端 | Go + 标准库 `net/http`（不引入 Web 框架） |
| 数据库 | MySQL 8.0 |
| 入口 / 部署 | Nginx（静态资源 + `/api` 反代）+ Docker Compose |

请求链路只有一条：**浏览器 → Nginx → Go → MySQL**。

## 目录结构

```
backend/     Go API：config / db / auth / httpx / materials / knowledge / server
frontend/    React + TypeScript + Vite 前端
deploy/      nginx.conf（容器内）；nginx.local.conf（本机裸机验证用）
openspec/    change 四件套与主力规约
docs/        迭代说明与验收证据
homework/    0923 作业：文件下载权限的分析与截图证据
scripts/     本地验证脚本
```

## 接口一览

| 方法 | 路径 | 鉴权 | 成功 | 失败 |
| --- | --- | --- | --- | --- |
| GET | `/health` | 公开 | 200 | — |
| POST | `/api/login` | 公开 | 200 + Set-Cookie | 401（统一文案，含锁定期） |
| POST | `/api/logout` | 会话 | 204 | 401 |
| GET | `/api/me` | 会话 | 200 | 401 |
| GET | `/api/materials` | 会话 | 200（仅本班） | 401 |
| POST | `/api/materials` | 教师 | 201 | 401 / 403 / 400 / 413 |
| GET | `/api/materials/{id}` | 会话 | 200（本班） | 401 / 404 |
| GET | `/api/materials/{id}/file` | 会话 | 200（附件） | 401 / 404 |

## 快速开始（Docker Compose，标准启动方式）

```bash
cp .env.example .env
# 填写 SESSION_SECRET、DB_ROOT_PASSWORD、DB_PASSWORD、三个 SEED_*_PASSWORD
# 生成密钥示例：openssl rand -hex 32

docker compose up --build -d
docker compose ps
# 打开 http://localhost:8080
```

只有 **web** 映射宿主端口（默认 8080）；`db` 不映射 3306，`api` 不单独暴露。
数据放在 volume 里：`docker compose down` 后再 `up -d`（**不加 `-v`**）数据仍在；`-v` 只用于主动重置。

预置账号（口令来自 `.env` 中的 `SEED_*_PASSWORD`）：

| 账号 | 角色 | 班级 |
| --- | --- | --- |
| `teacher_a` | 教师 | A 班 |
| `student_a1` | 学生 | A 班 |
| `student_b1` | 学生 | B 班 |

登录页：`http://localhost:8080/`　存活探针：`http://localhost:8080/health`（无需登录）

## 本机裸机运行（没有 Docker 时用于验证）

```bash
brew install go mysql nginx && brew services start mysql

mysql -u root -e "CREATE DATABASE IF NOT EXISTS campusclaw CHARACTER SET utf8mb4;
CREATE USER IF NOT EXISTS 'campusclaw_user'@'127.0.0.1' IDENTIFIED BY '<你的口令>';
GRANT ALL PRIVILEGES ON campusclaw.* TO 'campusclaw_user'@'127.0.0.1'; FLUSH PRIVILEGES;"

cp .env.example .env   # 把 DB_HOST 改成 127.0.0.1
set -a && source .env && set +a
(cd backend && go build -o ../bin/server ./cmd/server && ../bin/server &)

(cd frontend && npm install && npm run build)
nginx -c "$PWD/deploy/nginx.local.conf" -p "$PWD/"
# 打开 http://localhost:8088
```

`deploy/nginx.local.conf` 与容器内的 `deploy/nginx.conf` 规则一致，只是监听端口与上游地址换成本机地址。

## 测试

```bash
cd backend && go test ./...        # 单元测试：配置校验、限流、口令哈希、上传校验
cd frontend && npm run build       # TypeScript 类型检查 + 构建
```

## 安全说明

- 所有必填配置来自环境变量，缺 `SESSION_SECRET` 或数据库凭据时**启动失败**，不退回内置默认值；
  `.env` 不入库也不进镜像，`.env.example` 列出全部必填项。
- 口令以 bcrypt 存储；会话标识在服务端以「HMAC(会话 token)」形式保存。
- 上传白名单 `.txt` / `.md`，存储名由服务端生成，客户端文件名只作展示标题。
- 上传目录只挂载到 api，Nginx 侧显式 `location /uploads/ { return 404; }`。
- 认证失败统一文案并对「用户名 + 客户端 IP」限流；锁定期的响应与凭据错误一致，不用 429 暴露账号存在。
- Markdown 渲染不解析内联 HTML，材料里的 `<script>` 只以文本呈现。

## 相关文档

- `homework/0923-文件下载权限/作业-0923-文件下载是否需要权限.md`：文件下载权限的分析与截图证据
- `docs/iteration-1.md`：迭代说明与四项设计决策（ADR）
- `docs/evidence.md`：主路径与失败路径的验收证据
- `openspec/`：本变更的 proposal / design / tasks / delta spec
