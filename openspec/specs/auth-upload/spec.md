# auth-upload Specification

## Purpose

本能力规定 CampusClaw 底座的身份、权限与数据边界：账号密码登录与服务端会话、教师 / 学生两种角色的能力边界、
以班级为租户的数据隔离，以及材料的读取与文件下载必须经过服务端鉴权。

本能力同时规定材料上传与知识库正文入库的校验与事务要求：白名单扩展名、大小上限、UTF-8 校验、
服务端生成存储名，`materials` 与 `knowledge_entries` 同事务写入，失败不留残留。

知识库在本能力中只负责保存可查询的正文，不包含检索与问答（见对应变更的 Non-goals）。

## Requirements

### Requirement: R1 登录与服务端会话

系统 MUST 提供账号密码登录，并在服务端维护会话状态。会话 MUST 能判定 `user_id`、`role`、`class_id` 三项。

#### Scenario: R1.1 正确凭据登录成功

- **WHEN** 客户端 `POST /api/login` 提交 `{"username":"teacher_a","password":"<预置口令>"}`
- **THEN** 响应状态码为 200
- **AND** 响应体包含 `user_id`、`role=teacher`、`class_id`
- **AND** 响应 `Set-Cookie` 携带服务端会话标识

#### Scenario: R1.2 认证失败统一响应

- **WHEN** 客户端提交不存在的用户名，或提交已存在用户名但口令错误
- **THEN** 两种情况返回相同的状态码 401 与逐字节一致的响应体
- **AND** 响应体 MUST NOT 包含材料标题、正文或磁盘路径
- **AND** 处于锁定期时的响应与凭据错误的响应一致，MUST NOT 通过 429 或差异文案暴露账号是否存在

#### Scenario: R1.3 登录换发新的会话标识（防会话固定）

- **WHEN** 客户端携带一个登录前的会话 Cookie 提交正确凭据并登录成功
- **THEN** 服务端签发新的会话标识，登录前的会话行 MUST 失效
- **AND** 使用登录前的旧标识再次请求受保护接口时返回 401

#### Scenario: R1.4 Cookie 属性

- **WHEN** 登录成功返回 `Set-Cookie`
- **THEN** Cookie MUST 带 `HttpOnly` 与 `SameSite=Lax`
- **AND** 本机 HTTP 环境下 MUST NOT 强制 `Secure`

### Requirement: R2 角色与能力边界

系统 MUST 只实现 `teacher` 与 `student` 两种角色，并在服务端按会话角色授权。
教师可上传、查看、下载本班材料；学生只能查看与下载。

#### Scenario: R2.1 学生上传被拒绝

- **WHEN** 持学生会话的客户端 `POST /api/materials` 上传一个合法的 `.txt`
- **THEN** 响应状态码为 403
- **AND** `materials` 与 `knowledge_entries` 行数保持不变
- **AND** 上传目录不新增任何文件

#### Scenario: R2.2 教师上传成功

- **WHEN** 持教师会话的客户端上传合法的 `.txt` 或 `.md`
- **THEN** 响应状态码为 201
- **AND** 返回新建材料的 `id` 与展示标题

#### Scenario: R2.3 角色不采信客户端声明

- **WHEN** 持学生会话的客户端在上传请求中附加 `role=teacher`（表单字段或请求头）
- **THEN** 服务端仍按会话角色处理，返回 403

#### Scenario: R2.4 身份接口

- **WHEN** 已登录客户端 `GET /api/me`
- **THEN** 返回 200 与 `user_id`、`username`、`role`、`class_id`
- **AND** 未登录时同一接口返回 401

### Requirement: R3 班级隔离（租户边界）

每个用户 MUST 属于一个固定班级。租户标识 MUST 只来自服务端会话，请求参数中的班级字段 MUST 被忽略。

#### Scenario: R3.1 跨班按 ID 与不存在同形 404

- **WHEN** 持 A 班会话的客户端 `GET /api/materials/{id}`，`id` 为 B 班材料 ID
- **AND** 客户端再请求一个不存在的 ID
- **THEN** 两次响应的状态码均为 404，响应体逐字节一致
- **AND** 两次响应 MUST NOT 包含对方的标题、正文、存储名或磁盘路径
- **AND** 服务端日志记录真实原因以区分越权试探与误输

#### Scenario: R3.2 列表按会话班级过滤

- **WHEN** 持 A 班会话的客户端 `GET /api/materials`
- **THEN** 返回的每一条都满足 `class_id = A`
- **AND** 结果来自数据库查询，MUST NOT 为硬编码条目
- **AND** 清空 `materials` 表后再次请求，返回空列表

#### Scenario: R3.3 上传归属取自会话

- **WHEN** 持 A 班教师会话的客户端上传材料，且表单中附带 `class_id=B`
- **THEN** 新建的 `materials` 与 `knowledge_entries` 行的 `class_id` 均为 A

#### Scenario: R3.4 列表不接受客户端指定班级

- **WHEN** 持 A 班会话的客户端 `GET /api/materials?class_id=B`
- **THEN** 返回结果仍只有 A 班材料

### Requirement: R4 上传校验与知识库入库

上传 MUST 先校验再落盘，并在同一事务内写入 `materials` 与 `knowledge_entries`。

#### Scenario: R4.1 扩展名白名单

- **WHEN** 客户端上传扩展名不属于 `.txt` / `.md` 的文件（例如 `.exe`、`.md.exe`）
- **THEN** 响应状态码为 400
- **AND** `materials` 与 `knowledge_entries` 无新增行，上传目录无新增文件

#### Scenario: R4.2 成功入库两表各一行

- **WHEN** 教师上传合法的 `.txt` 或 `.md`
- **THEN** `materials` 新增一行，`knowledge_entries` 新增一行并通过 `material_id` 关联
- **AND** 两行 `class_id` 均等于上传者会话所在班级
- **AND** `knowledge_entries.content` 为该文件解析出的正文

#### Scenario: R4.3 存储名由服务端生成

- **WHEN** 客户端提交的文件名包含 `../` 或以路径分隔符开头
- **THEN** 服务端自行生成存储名，落盘路径仍在配置的上传目录内
- **AND** 客户端文件名只作为展示标题保存

#### Scenario: R4.4 超出大小上限

- **WHEN** 上传体积超过 `MAX_UPLOAD_BYTES` 的文件
- **THEN** 响应状态码为 413
- **AND** 服务端在读完整个请求体之前即拒绝
- **AND** 数据库与上传目录均无残留

#### Scenario: R4.5 内容非法

- **WHEN** 上传空文件或内容不是合法 UTF-8 文本
- **THEN** 响应状态码为 400
- **AND** 数据库与上传目录均无残留

#### Scenario: R4.6 失败不留孤儿

- **WHEN** 上传过程中磁盘写入或数据库写入任一步失败
- **THEN** 事务回滚，已写入的文件被删除
- **AND** 系统回到"无文件、无记录"的干净状态

### Requirement: R5 材料读取与文件访问

材料详情与文件 MUST 经过鉴权与班级核对，文件 MUST NOT 通过静态路径暴露。

#### Scenario: R5.1 本班详情可读

- **WHEN** 持本班会话的客户端 `GET /api/materials/{id}`
- **THEN** 返回 200 与标题、正文、上传者、创建时间

#### Scenario: R5.2 下载走鉴权接口

- **WHEN** 持本班会话的客户端 `GET /api/materials/{id}/file`
- **THEN** 返回 200 与文件内容，`Content-Disposition` 为附件并带原始文件名
- **AND** 未登录时同一路径返回 401

#### Scenario: R5.3 上传目录不静态暴露

- **WHEN** 客户端直接请求 `/uploads/<猜测的文件名>`
- **THEN** 请求不返回该文件内容

### Requirement: R6 登出

系统 MUST 提供登出能力：登出时删除服务端会话行并清除浏览器 Cookie，使旧会话标识立即失效。

#### Scenario: R6.1 登出删除服务端会话

- **WHEN** 已登录客户端 `POST /api/logout`
- **THEN** 响应状态码为 204，`sessions` 中该行被删除，Cookie 被清除

#### Scenario: R6.2 旧 Cookie 失效

- **WHEN** 登出后使用登出前的 Cookie 再次请求 `GET /api/materials`
- **THEN** 返回 401

### Requirement: R7 配置与密钥

所有密钥、数据库凭据与种子口令 MUST 来自环境变量；仓库中 MUST NOT 出现真实可用的密钥，缺失必填项时进程 MUST 启动失败。

#### Scenario: R7.1 配置外置且示例齐全

- **WHEN** 第三方克隆仓库并阅读 `.env.example`
- **THEN** 其中列出全部必填环境变量且不含真实值
- **AND** 仓库中不存在真实可用的密钥或口令

#### Scenario: R7.2 口令哈希存储

- **WHEN** 查询 `users.password_hash`
- **THEN** 字段为 bcrypt 哈希（形如 `$2a$` / `$2b$` 前缀），MUST NOT 为明文或 MD5/SHA 无盐哈希
- **AND** 种子口令来自环境变量

#### Scenario: R7.3 缺必填配置启动失败

- **WHEN** 缺少 `SESSION_SECRET` 或数据库凭据时启动 api
- **THEN** 进程以非零状态退出并打印缺失项名称
- **AND** MUST NOT 退回内置默认密钥

### Requirement: R8 数据模型与幂等种子

系统 MUST 建立 `classes`、`users`、`sessions`、`materials`、`knowledge_entries` 五张表，并提供可重复执行的种子数据；`materials` 与 `knowledge_entries` 的 `class_id` MUST 非空并建立索引。

#### Scenario: R8.1 种子幂等

- **WHEN** 连续启动两次
- **THEN** 用户与种子材料行数不增加，教师已上传的材料不被覆盖

#### Scenario: R8.2 两班材料标题可区分

- **WHEN** 阅读种子数据
- **THEN** A 班与 B 班各至少有一条标题可区分的材料，并各关联一条知识库正文
- **AND** 用 B 班标题在 A 班列表中搜索时应无结果

#### Scenario: R8.3 班级字段约束

- **WHEN** 查看 `materials` 与 `knowledge_entries` 建表语句
- **THEN** `class_id` 为 NOT NULL 且建立索引
- **AND** 尝试写入空 `class_id` 时被数据库拒绝

### Requirement: R9 最小 UI 与未授权行为

系统 MUST 提供登录页与材料页两个页面；身份 MUST 以 `GET /api/me` 为准，前端 MUST NOT 承担访问控制。

#### Scenario: R9.1 登录页

- **WHEN** 未登录用户打开站点根路径
- **THEN** 看到登录页，提交表单调用 `/api/login`

#### Scenario: R9.2 材料列表来自本班查询

- **WHEN** 已登录用户进入材料页
- **THEN** 列表调用 `GET /api/materials`，数据来自数据库按班级过滤的查询

#### Scenario: R9.3 上传入口仅教师可见

- **WHEN** 身份由 `GET /api/me` 恢复
- **THEN** 只有 `role=teacher` 时渲染上传入口
- **AND** 手工修改前端本地状态不改变服务端判定：绕过界面直接调用上传接口仍按 R2.1 被拒

#### Scenario: R9.4 详情与下载走鉴权 API

- **WHEN** 用户打开材料详情或点击下载
- **THEN** 请求 `/api/materials/{id}` 与 `/api/materials/{id}/file`，MUST NOT 直接引用上传目录的静态 URL

#### Scenario: R9.5 401 回到登录页

- **WHEN** 任一受保护请求返回 401
- **THEN** 前端清空本地状态并回到登录页

#### Scenario: R9.6 正文渲染不执行脚本

- **WHEN** 上传内容包含 `<script>` 或 `<img onerror>` 的 `.md`
- **THEN** 详情页以文本形式呈现这些片段，浏览器 MUST NOT 执行其中的脚本

#### Scenario: R9.7 本班范围搜索

- **WHEN** 用户在材料页搜索 B 班材料的标题
- **THEN** 结果为空（搜索只在本班已加载的数据范围内过滤）

### Requirement: R10 页面体验增强

系统 MUST 提供品牌色与响应式布局、深浅色主题、列表 / 网格视图、命令面板、上传进度、操作反馈与 GFM Markdown 渲染这些写入规约的体验条目。

以下为写入规约后必须验收的体验条目；它们 MUST NOT 改变 401 / 403 / 404 的服务端语义。

#### Scenario: R10.1 品牌与响应式

- **WHEN** 在桌面与移动宽度打开页面
- **THEN** 布局自适应且使用统一品牌色

#### Scenario: R10.2 深浅色主题

- **WHEN** 切换主题开关
- **THEN** 页面在浅色与深色之间切换，偏好被保留

#### Scenario: R10.3 列表与网格视图

- **WHEN** 切换列表 / 网格
- **THEN** 材料以对应布局呈现

#### Scenario: R10.4 命令面板

- **WHEN** 按下 `⌘/Ctrl + K`
- **THEN** 打开命令面板，可跳转材料或执行登出

#### Scenario: R10.5 上传进度

- **WHEN** 教师上传文件
- **THEN** 界面显示上传进度，完成后刷新本班列表

#### Scenario: R10.6 操作反馈

- **WHEN** 登录失败、上传失败或上传成功
- **THEN** 通过页内提示或 toast 反馈结果，登录失败只显示统一文案

#### Scenario: R10.7 Markdown 渲染

- **WHEN** 打开 `.md` 材料详情
- **THEN** 使用 GFM 渲染（表格、任务列表等），且 HTML 按 R9.6 转义
