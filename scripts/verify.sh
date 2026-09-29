#!/usr/bin/env bash
# 迭代 1 快速验收：主路径与三条失败路径。任一不符合预期即退出非零。
# 用法：BASE_URL=http://localhost:8080 TEACHER_PW=... STUDENT_A_PW=... STUDENT_B_PW=... ./scripts/verify.sh
#
# 注意：材料 id 从接口动态发现，不写死——重新播种或清库后 id 会变化。
set -u
BASE_URL="${BASE_URL:-http://localhost:8080}"
TEACHER_PW="${TEACHER_PW:?请通过环境变量提供预置教师口令}"
STUDENT_A_PW="${STUDENT_A_PW:-${STUDENT_PW:-}}"
STUDENT_B_PW="${STUDENT_B_PW:?请通过环境变量提供 B 班预置学生口令}"
[ -n "$STUDENT_A_PW" ] || { echo "请提供 STUDENT_A_PW（或 STUDENT_PW）"; exit 2; }

JAR_TEACHER="$(mktemp)"; JAR_A="$(mktemp)"; JAR_B="$(mktemp)"
TMP_MD="$(mktemp -t campusclaw-upload).md"
printf '# 验收用材料\n\n正文内容\n' > "$TMP_MD"
FAILED=0

check() { if [ "$2" = "$3" ]; then printf '  ✔ %s（%s）\n' "$1" "$3"; else printf '  ✘ %s：期望 %s，实际 %s\n' "$1" "$2" "$3"; FAILED=1; fi; }
login() { curl -s -c "$1" -o /dev/null -X POST "$BASE_URL/api/login" -H 'Content-Type: application/json' -d "{\"username\":\"$2\",\"password\":\"$3\"}"; }
first_material_id() { curl -s -b "$1" "$BASE_URL/api/materials" | python3 -c 'import sys,json;i=json.load(sys.stdin)["items"];print(i[0]["id"] if i else "")'; }

login "$JAR_TEACHER" teacher_a "$TEACHER_PW"
login "$JAR_A" student_a1 "$STUDENT_A_PW"
login "$JAR_B" student_b1 "$STUDENT_B_PW"

echo "1. 未登录"
check "GET /health 无需登录" 200 "$(curl -s -o /dev/null -w '%{http_code}' "$BASE_URL/health")"
check "GET /api/materials 返回 401" 401 "$(curl -s -o /dev/null -w '%{http_code}' "$BASE_URL/api/materials")"
check "GET /uploads/<猜测文件名> 不暴露文件" 404 "$(curl -s -o /dev/null -w '%{http_code}' "$BASE_URL/uploads/seed-a-class-reading.md")"

echo "2. 登录与会话"
check "教师 /api/me" 200 "$(curl -s -o /dev/null -w '%{http_code}' -b "$JAR_TEACHER" "$BASE_URL/api/me")"
check "学生A /api/me" 200 "$(curl -s -o /dev/null -w '%{http_code}' -b "$JAR_A" "$BASE_URL/api/me")"

MAT_A="$(first_material_id "$JAR_A")"
MAT_B="$(first_material_id "$JAR_B")"
[ -n "$MAT_A" ] && [ -n "$MAT_B" ] || { echo "未取到两班材料（种子数据缺失？）"; exit 2; }
echo "  · A 班材料 id=${MAT_A}，B 班材料 id=$MAT_B"

echo "3. 主路径"
check "教师上传 .md 返回 201" 201 "$(curl -s -o /dev/null -w '%{http_code}' -b "$JAR_TEACHER" -X POST "$BASE_URL/api/materials" -F "file=@$TMP_MD")"
check "学生下载本班文件返回 200" 200 "$(curl -s -o /dev/null -w '%{http_code}' -b "$JAR_A" "$BASE_URL/api/materials/$MAT_A/file")"
check "学生查看本班详情返回 200" 200 "$(curl -s -o /dev/null -w '%{http_code}' -b "$JAR_A" "$BASE_URL/api/materials/$MAT_A")"

echo "4. 失败路径"
check "未登录请求详情返回 401" 401 "$(curl -s -o /dev/null -w '%{http_code}' "$BASE_URL/api/materials/$MAT_A")"
check "学生上传返回 403（垂直越权）" 403 "$(curl -s -o /dev/null -w '%{http_code}' -b "$JAR_A" -X POST "$BASE_URL/api/materials" -F "file=@$TMP_MD")"
check "A 班学生访问 B 班材料返回 404（水平越权）" 404 "$(curl -s -o /dev/null -w '%{http_code}' -b "$JAR_A" "$BASE_URL/api/materials/$MAT_B/file")"
check "不存在的 ID 同样 404" 404 "$(curl -s -o /dev/null -w '%{http_code}' -b "$JAR_A" "$BASE_URL/api/materials/999999/file")"

CROSS="$(curl -s -b "$JAR_A" "$BASE_URL/api/materials/$MAT_B/file")"
MISSING="$(curl -s -b "$JAR_A" "$BASE_URL/api/materials/999999/file")"
if [ "$CROSS" = "$MISSING" ]; then
  printf '  ✔ 跨班与不存在的响应体逐字节一致：%s\n' "$CROSS"
else
  printf '  ✘ 跨班与不存在响应体不一致：%s / %s\n' "$CROSS" "$MISSING"; FAILED=1
fi

echo "5. 登出"
check "登出返回 204" 204 "$(curl -s -o /dev/null -w '%{http_code}' -b "$JAR_A" -X POST "$BASE_URL/api/logout")"
check "登出后旧 Cookie 返回 401" 401 "$(curl -s -o /dev/null -w '%{http_code}' -b "$JAR_A" "$BASE_URL/api/materials")"

[ "$FAILED" -eq 0 ] && echo "全部通过 ✔" || echo "存在未通过项 ✘"
exit "$FAILED"
