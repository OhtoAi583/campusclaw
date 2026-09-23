#!/usr/bin/env bash
# 快速验收脚本：跑通主路径与三条失败路径，任一不符合预期即退出非零。
# 用法：BASE_URL=http://localhost:8080 TEACHER_PW=... STUDENT_PW=... ./scripts/verify.sh
set -u
BASE_URL="${BASE_URL:-http://localhost:8080}"
TEACHER_PW="${TEACHER_PW:?请通过环境变量提供预置教师口令}"
STUDENT_PW="${STUDENT_PW:?请通过环境变量提供预置学生口令}"
JAR_TEACHER="$(mktemp)"
JAR_STUDENT="$(mktemp)"
TMP_MD="$(mktemp -t campusclaw-upload).md"
printf '# 验收用材料\n\n正文内容\n' > "$TMP_MD"
FAILED=0

check() { # check <描述> <期望> <实际>
  if [ "$2" = "$3" ]; then
    printf '  ✔ %s（%s）\n' "$1" "$3"
  else
    printf '  ✘ %s：期望 %s，实际 %s\n' "$1" "$2" "$3"
    FAILED=1
  fi
}

echo "1. 未登录"
check "GET /health 无需登录" 200 "$(curl -s -o /dev/null -w '%{http_code}' "$BASE_URL/health")"
check "GET /api/materials 返回 401" 401 "$(curl -s -o /dev/null -w '%{http_code}' "$BASE_URL/api/materials")"
check "GET /api/materials/1/file 返回 401" 401 "$(curl -s -o /dev/null -w '%{http_code}' "$BASE_URL/api/materials/1/file")"
check "GET /uploads/<猜测文件名> 不暴露文件" 404 "$(curl -s -o /dev/null -w '%{http_code}' "$BASE_URL/uploads/seed-a-class-reading.md")"

echo "2. 登录"
curl -s -c "$JAR_TEACHER" -o /dev/null -X POST "$BASE_URL/api/login" \
  -H 'Content-Type: application/json' -d "{\"username\":\"teacher_a\",\"password\":\"$TEACHER_PW\"}"
curl -s -c "$JAR_STUDENT" -o /dev/null -X POST "$BASE_URL/api/login" \
  -H 'Content-Type: application/json' -d "{\"username\":\"student_a1\",\"password\":\"$STUDENT_PW\"}"
check "教师 /api/me" 200 "$(curl -s -o /dev/null -w '%{http_code}' -b "$JAR_TEACHER" "$BASE_URL/api/me")"
check "学生 /api/me" 200 "$(curl -s -o /dev/null -w '%{http_code}' -b "$JAR_STUDENT" "$BASE_URL/api/me")"

echo "3. 主路径"
check "教师上传 .md 返回 201" 201 "$(curl -s -o /dev/null -w '%{http_code}' -b "$JAR_TEACHER" -X POST "$BASE_URL/api/materials" -F "file=@$TMP_MD")"
check "学生下载本班文件返回 200" 200 "$(curl -s -o /dev/null -w '%{http_code}' -b "$JAR_STUDENT" "$BASE_URL/api/materials/1/file")"

echo "4. 失败路径"
check "学生上传返回 403（垂直越权）" 403 "$(curl -s -o /dev/null -w '%{http_code}' -b "$JAR_STUDENT" -X POST "$BASE_URL/api/materials" -F "file=@$TMP_MD")"
check "学生访问 B 班材料返回 404（水平越权）" 404 "$(curl -s -o /dev/null -w '%{http_code}' -b "$JAR_STUDENT" "$BASE_URL/api/materials/2/file")"
check "不存在的 ID 同样 404" 404 "$(curl -s -o /dev/null -w '%{http_code}' -b "$JAR_STUDENT" "$BASE_URL/api/materials/999999/file")"

CROSS="$(curl -s -b "$JAR_STUDENT" "$BASE_URL/api/materials/2/file")"
MISSING="$(curl -s -b "$JAR_STUDENT" "$BASE_URL/api/materials/999999/file")"
if [ "$CROSS" = "$MISSING" ]; then
  printf '  ✔ 跨班与不存在的响应体逐字节一致：%s\n' "$CROSS"
else
  printf '  ✘ 跨班与不存在响应体不一致：%s / %s\n' "$CROSS" "$MISSING"
  FAILED=1
fi

echo "5. 登出"
check "登出返回 204" 204 "$(curl -s -o /dev/null -w '%{http_code}' -b "$JAR_STUDENT" -X POST "$BASE_URL/api/logout")"
check "登出后旧 Cookie 返回 401" 401 "$(curl -s -o /dev/null -w '%{http_code}' -b "$JAR_STUDENT" "$BASE_URL/api/materials")"

if [ "$FAILED" -eq 0 ]; then
  echo "全部通过 ✔"
else
  echo "存在未通过项 ✘"
fi
exit "$FAILED"
