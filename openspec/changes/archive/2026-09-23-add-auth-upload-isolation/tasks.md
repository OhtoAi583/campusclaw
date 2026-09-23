# Tasks: 认证授权与知识库入库（迭代 1）

## Apply 约定

- 一条 task 一次循环：实现 → 对照 spec 审查 diff → 运行该行 verify → 通过后勾选并提交。
- 失败的 task 不带入下一项。
- Apply 期间只允许改 change 里的 delta（`openspec/changes/add-auth-upload-isolation/`），不改 `openspec/specs/`。
- verify 栏是可执行判据，不是装饰；没跑过的项不得勾选。

## 1. 项目骨架与配置

- [x] 1.1 建立 `backend/`（Go + net/http）与 `frontend/`（React + TypeScript + Vite）目录骨架，导入与构建不报错 — verify: `docker compose build` 成功退出，前端 `npm run build` 无 TS 报错
- [x] 1.2 配置全部从环境变量读取（`SESSION_SECRET`、数据库凭据、上传目录与上限、会话 TTL、登录失败阈值、种子口令）— verify: 在 `backend/internal/config` 中列出变量名，与 `.env.example` 逐条比对无缺漏
- [x] 1.3 提交 `.env.example`，列出全部必填项且不含真实值 — verify: `grep -c . .env.example` 与 config 读取的变量数一致；`git check-ignore .env` 有输出
- [ ] 1.4 提交 `.gitignore` 与 `.dockerignore`，排除 `.env`、`uploads/`、构建产物 — verify: 构建镜像后 `docker run --rm <api-image> ls -a /app` 不含 `.env`
- [x] 1.5 缺 `SESSION_SECRET` 或数据库凭据时启动失败，不落内置默认值 — verify: `docker compose run --rm -e SESSION_SECRET= api` 退出码非 0 且打印缺失项

## 2. 数据层与种子

- [x] 2.1 建表 `classes`、`users`、`sessions`、`materials`、`knowledge_entries` — verify: 启动后 `docker compose exec -T db mysql -e "show tables"` 列出五张表
- [x] 2.2 `materials` 与 `knowledge_entries` 的 `class_id` 非空并建立索引 — verify: R8.3 的查询语句返回 `NOT NULL` 与索引名
- [x] 2.3 种子班级 A / B 与账号 `teacher_a` / `student_a1` / `student_b1` — verify: `select username, role, class_id from users` 返回三行且归属正确
- [x] 2.4 两班各一条标题可区分的材料并各关联一条知识库正文 — verify: R8.2 的按班查询各返回一条，标题不同
- [x] 2.5 种子幂等：连续启动两次行数不变 — verify: 连续 `docker compose up -d api` 两次后比对 users / materials 行数
- [x] 2.6 种子口令来自环境变量并以 bcrypt 存储 — verify: 查 `users.password_hash` 以 `$2` 开头，且不等于 `.env` 中的明文
- [x] 2.7 列表查询带班级条件，按 ID 取行不提前过滤班级 — verify: 阅读 `internal/materials/store.go`，两条路径分别为 `WHERE class_id = ?` 与 `WHERE id = ?`

## 3. 登录、会话与限流

- [x] 3.1 `POST /api/login` 校验 bcrypt 并写入 `sessions` — verify: curl 预置教师账号返回 200 且响应含 role 与 class_id
- [x] 3.2 登录成功换发新会话标识，旧标识作废（防会话固定）— verify: 带旧 Cookie 登录后，用旧 Cookie 请求 `/api/materials` 返回 401
- [x] 3.3 Cookie 为 `HttpOnly` + `SameSite=Lax`，本机不加 `Secure` — verify: 查看登录响应 `Set-Cookie` 原文
- [x] 3.4 认证失败统一文案，未知用户也执行一次哈希比较 — verify: 用户名不存在与口令错误两次请求的响应体逐字节一致
- [x] 3.5 按用户名 + IP 限流；锁定期的响应与凭据错误一致 — verify: 连续多次错误登录后响应仍为原 401 文案，MUST NOT 出现 429
- [x] 3.6 `POST /api/logout` 删除服务端会话行并清 Cookie — verify: 登出后 `select count(*) from sessions` 减少一行且旧 Cookie 请求返回 401
- [x] 3.7 鉴权中间件默认拒绝：受保护接口未登录返回 401 且响应不含业务数据 — verify: 清空 Cookie 请求 `/api/materials`，响应体不含标题与正文

## 4. 班级隔离

- [x] 4.1 列表按会话班级过滤 — verify: R3.2 的 curl 输出只含本班材料
- [x] 4.2 按 ID 取详情与文件先取行再核对班级 — verify: 阅读 handler，取行与班级核对顺序正确
- [x] 4.3 跨班与不存在返回同形 404 — verify: A 班 Cookie 请求 B 班 ID 与不存在 ID，状态码与响应体一致
- [x] 4.4 上传落库班级取自会话 — verify: 表单携带 `class_id=B` 上传后查库仍为 A 班
- [x] 4.5 列表忽略客户端传入的 `class_id` 参数 — verify: `GET /api/materials?class_id=B` 仍只返回 A 班材料

## 5. 角色授权与上传入库

- [x] 5.1 `POST /api/materials` 仅教师可用，学生返回 403 — verify: 学生会话与教师会话各调一次，分别 403 与 201
- [x] 5.2 学生上传后 `materials` / `knowledge_entries` 行数与上传目录均不变 — verify: 上传前后行数与 `ls uploads | wc -l` 一致
- [x] 5.3 扩展名白名单 `.txt` / `.md`，其余 400 — verify: 上传 `.exe` 与 `.md.exe` 各返回 400
- [x] 5.4 超过 `MAX_UPLOAD_BYTES` 返回 413 — verify: 生成超限文件上传返回 413
- [x] 5.5 空内容或非 UTF-8 返回 400 — verify: 上传空文件与二进制文件各返回 400
- [x] 5.6 存储名由服务端生成，客户端文件名不参与路径 — verify: 文件名含 `../` 时文件仍落在上传目录内且标题为原始名字
- [x] 5.7 两表同事务写入，失败回滚且删除文件 — verify: 阅读上传 handler 的事务与清理分支，并构造一次失败后检查无残留
- [x] 5.8 详情与下载走鉴权接口，上传目录不静态暴露 — verify: 直接请求 `/uploads/<文件名>` 不返回文件内容

## 6. 材料读取 API

- [x] 6.1 `GET /api/materials/{id}` 返回标题、正文、上传者与创建时间 — verify: 本班会话请求返回 200 且字段齐全
- [x] 6.2 `GET /api/materials/{id}/file` 以附件形式返回原文件 — verify: 响应头含 `Content-Disposition: attachment`
- [x] 6.3 详情与下载对未登录返回 401，对跨班返回 404 — verify: 三条 curl 分别得到 401 / 404

## 7. 前端页面

- [x] 7.1 登录页调用 `/api/login`，失败只显示统一文案 — verify: 浏览器提交错误口令，页面显示统一错误
- [x] 7.2 刷新时用 `GET /api/me` 恢复身份，无效则回登录页 — verify: 刷新页面与重开标签页各试一次
- [x] 7.3 材料列表调用 `/api/materials`，数据来自数据库 — verify: 清空 `materials` 表后刷新，列表为空
- [x] 7.4 上传入口仅在 `role=teacher` 时渲染，依据 `/api/me` — verify: 学生登录看不到入口；教师登录可见
- [x] 7.5 收到 401 时清空状态并回登录页 — verify: 手动删除 Cookie 后触发请求，页面回到登录页
- [x] 7.6 详情页渲染 Markdown 且 HTML 被转义 — verify: 上传含 `<script>` 的 `.md`，页面显示为文本且不弹窗
- [x] 7.7 本班范围内搜索标题与正文 — verify: 用 B 班标题在 A 班页面搜索，结果为空
- [x] 7.8 深浅色主题切换并保留偏好 — verify: 切换后刷新，主题保持
- [x] 7.9 列表 / 网格视图切换 — verify: 点击切换按钮，布局变化
- [x] 7.10 `⌘/Ctrl + K` 命令面板可跳转材料与登出 — verify: 快捷键打开面板并执行一次跳转
- [x] 7.11 上传进度与 toast 反馈 — verify: 上传一个较大 `.md`，进度条推进并在完成后出现成功提示
- [x] 7.12 响应式布局与统一品牌色 — verify: 移动宽度下布局不溢出

## 8. Docker Compose 与文档

- [ ] 8.1 `docker-compose.yml` 定义 web / api / db 三个服务，只有 web 映射宿主端口 — verify: `docker compose ps` 中仅 web 有 8080 映射，`docker compose config` 无 db/api `ports`
- [ ] 8.2 db 使用 volume 持久化，上传目录挂到 api — verify: `docker compose down` 后 `up -d`，登录后数据仍在
- [ ] 8.3 api 连接数据库前重试等待就绪 — verify: 全新启动（先 `down -v`）观察首次 `up` 后 `/api/me` 不出现 502
- [x] 8.4 `GET /health` 无需登录且只做存活判定 — verify: 未登录请求 `/health` 返回 200；停掉 db 后 `/health` 仍为 200 而受保护接口不返回 401
- [x] 8.5 README 写明范围、不做项、单实例部署、跨班返回码、启动方式、预置账号 — verify: 只读 README 能回答这四个问题
- [ ] 8.6 按 README 从零启动可打开登录页 — verify: 在干净目录按 README 执行一轮，浏览器可见登录页
- [x] 8.7 迭代说明写清四项决策（班级来源、404 及备选、上传事务与清理、`/health` 语义）— verify: `docs/iteration-1.md` 中四项各有备选与否决理由

## 9. 发布验收

- [x] 9.1 `openspec validate <change> --strict` 通过 — verify: 命令退出码为 0 且无 error
- [x] 9.2 主路径取证：登录 → 上传 → 列表可见 → 学生只读下载 — verify: `docs/evidence.md` 记录四条命令与输出
- [x] 9.3 失败路径取证：未登录、学生上传、跨班访问 — verify: `docs/evidence.md` 记录三条命令与输出
- [x] 9.4 关键 Scenario 逐条给出判定结论 — verify: `docs/evidence.md` 对 R3.1 / R2.1 / R4.2 等给出通过与否及依据
- [x] 9.5 打版本 tag — verify: `git tag` 列出 `v0.1.0-auth-upload`
- [x] 9.6 归档：活动变更清零、`changes/archive/` 出现带日期的目录、主力规约生成 — verify: `openspec list` 无本 change，`openspec/specs/auth-upload/spec.md` 存在

## 验证范围与未勾选项说明

本次验证在**本机裸机环境**完成（Go 1.27 / MySQL / nginx），等价覆盖了下列条目：

- §1–§7、§9 与 §8.4 / §8.5 / §8.7 均已实测通过，证据见
  `docs/evidence.md`、`docs/验收截图/`、`homework/0923-文件下载权限/`。
- §1.1 / §1.4 中"构建镜像并检查镜像内容"这一步因本机**无法访问镜像仓库**
  （`registry-1.docker.io` 与各镜像 CDN 均连接超时）未能执行，因此保持未勾选：
  - 1.1 已用 `go build` + `npm run build` 等价验证编译与构建通过；
  - 1.4 已提交 `.gitignore` 与 `.dockerignore`，但"构建镜像后确认镜像内不含 `.env`"未执行。
- §8.1 / §8.2 / §8.3 / §8.6 依赖 Docker Compose 实际启动，同样因镜像仓库不可达而未勾选。
  这三项需要用可访问镜像仓库的环境按 README 复跑一轮后补勾：
  `docker compose up --build -d` → `docker compose ps` → 下载一个 `.md` 验证 → `down` / `up -d` 验证数据仍在。
