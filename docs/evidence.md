# 验收证据：主路径与失败路径

所有命令均在 `http://127.0.0.1:8088`（Nginx → Go → MySQL）上实测。
完整响应头与逐条输出见 `homework/0923-文件下载权限/原始请求记录.txt` 与同目录截图。

## 主路径

### 一、登录 → 上传 → 本班列表可见

```bash
# 1. 教师登录（写入会话 Cookie）
$ curl -s -c /tmp/t.jar -X POST http://127.0.0.1:8088/api/login \
    -H 'Content-Type: application/json' \
    -d '{"username":"teacher_a","password":"<预置口令>"}'
{"user_id":1,"username":"teacher_a","role":"teacher","class_id":1,"class_name":"A"}

# 2. 上传一个 .md
$ printf '# 新教研材料\n\n正文内容\n' > /tmp/lesson.md
$ curl -s -b /tmp/t.jar -X POST http://127.0.0.1:8088/api/materials -F "file=@/tmp/lesson.md"
{"id":3,"class_id":1,"title":"lesson",...,"uploader_name":"teacher_a",...}   # 201

# 3. 两表各新增一行（同事务）
$ docker compose exec -T db mysql -uroot -p"$DB_ROOT_PASSWORD" campusclaw -e \
    "select count(*) from materials; select count(*) from knowledge_entries;"

# 4. 本班列表能看到（数据来自数据库查询，不是硬编码）
$ curl -s -b /tmp/t.jar http://127.0.0.1:8088/api/materials
{"class_id":1,"items":[{...新上传的 lesson...},{...A 班种子材料...}]}

# 5. 清空材料表后刷新列表，应为空（证明列表不是写死的）
$ docker compose exec -T db mysql -uroot -p"$DB_ROOT_PASSWORD" campusclaw -e "delete from materials;"
$ curl -s -b /tmp/t.jar http://127.0.0.1:8088/api/materials
{"class_id":1,"items":[]}
```

### 二、同班学生：可浏览、可下载、不可上传

```bash
$ curl -s -c /tmp/s.jar -X POST http://127.0.0.1:8088/api/login \
    -H 'Content-Type: application/json' -d '{"username":"student_a1","password":"<预置口令>"}'
$ curl -s -o /dev/null -w "%{http_code}\n" -b /tmp/s.jar http://127.0.0.1:8088/api/materials/1/file
200        # 下载本班文件成功，Content-Disposition: attachment
```

## 失败路径（安全需求靠负向测试证明）

### 一、未登录访问受保护接口

```bash
$ curl -i http://127.0.0.1:8088/api/materials
HTTP/1.1 401 Unauthorized
{"error":"unauthorized"}          # 响应体不含任何标题、正文或磁盘路径

$ curl -i http://127.0.0.1:8088/api/materials/1/file
HTTP/1.1 401 Unauthorized
{"error":"unauthorized"}
```

### 二、学生上传（垂直越权）

```bash
$ curl -i -b /tmp/s.jar -X POST http://127.0.0.1:8088/api/materials -F "file=@/tmp/demo.md"
HTTP/1.1 403 Forbidden
{"error":"forbidden"}
# 上传前后 materials / knowledge_entries 行数与上传目录文件数均不变
```

### 三、跨班访问（水平越权）

```bash
# 学生 A 访问 B 班材料
$ curl -i -b /tmp/s.jar http://127.0.0.1:8088/api/materials/2/file
HTTP/1.1 404 Not Found
{"error":"not_found"}

# 不存在的 ID：响应必须完全同形
$ curl -i -b /tmp/s.jar http://127.0.0.1:8088/api/materials/999/file
HTTP/1.1 404 Not Found
{"error":"not_found"}
```

### 四、上传校验失败（扩展名 / 大小 / 内容）

```bash
$ curl -o /dev/null -w "%{http_code}\n" -b /tmp/t.jar -X POST http://127.0.0.1:8088/api/materials -F "file=@/tmp/payload.exe"
400
$ head -c 3000000 /dev/zero > /tmp/big.md && curl -o /dev/null -w "%{http_code}\n" -b /tmp/t.jar -X POST http://127.0.0.1:8088/api/materials -F "file=@/tmp/big.md"
413
$ curl -o /dev/null -w "%{http_code}\n" -b /tmp/t.jar -X POST http://127.0.0.1:8088/api/materials -F "file=@/tmp/binary.md"
400
# 三种失败后均无新增记录、上传目录无新增文件
```

### 五、`/health` 语义

```bash
$ curl -s -o /dev/null -w "%{http_code}\n" http://127.0.0.1:8088/health
200        # 无需登录；停掉数据库后仍为 200，受保护接口不再返回 401 而是 503
```

## 单元测试与构建

```bash
$ (cd backend && go test ./...)
ok  campusclaw/backend/internal/auth
ok  campusclaw/backend/internal/config
ok  campusclaw/backend/internal/materials

$ (cd frontend && npm run build)
✓ built in 553ms
```
