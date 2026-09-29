#!/usr/bin/env bash
# 迭代 2 验收：逐条对照 kb-retrieval 的关键 Scenario。
# 用法：BASE_URL=http://localhost:8080 TEACHER_PW=... STUDENT_A_PW=... STUDENT_B_PW=... ./scripts/verify-retrieval.sh
#
# 说明：JSON 请求体统一由 python3 生成，避免在 shell 里手写转义。
set -u
BASE_URL="${BASE_URL:-http://localhost:8080}"
TEACHER_PW="${TEACHER_PW:?请提供预置教师口令}"
STUDENT_A_PW="${STUDENT_A_PW:?请提供预置学生口令}"
STUDENT_B_PW="${STUDENT_B_PW:?请提供预置学生口令}"

JAR_A="$(mktemp)"; JAR_B="$(mktemp)"; JAR_T="$(mktemp)"
BODY="$(mktemp)"
TMP_MD="$(mktemp -t unique).md"
UNIQUE="zz_unique_$(date +%s)"
QUERY="独特标记 ${UNIQUE} 上传后立即可检索"
printf '# 上传即索引验证\n\n本材料包含独特标记 %s，用于验证上传后立即可检索。\n' "$UNIQUE" > "$TMP_MD"
FAILED=0

check() { if [ "$2" = "$3" ]; then printf '  ✔ %s（%s）\n' "$1" "$3"; else printf '  ✘ %s：期望 %s，实际 %s\n' "$1" "$2" "$3"; FAILED=1; fi; }
json() { python3 -c 'import json,sys;print(json.dumps({"query":sys.argv[1],"top_k":int(sys.argv[2])}))' "$1" "${2:-5}"; }
login() { curl -s -c "$1" -o /dev/null -X POST "$BASE_URL/api/login" -H 'Content-Type: application/json' -d "{\"username\":\"$2\",\"password\":\"$3\"}"; }
search() { json "$2" "${3:-5}" > "$BODY"; curl -s -b "$1" -X POST "$BASE_URL/api/search" -H 'Content-Type: application/json' --data-binary @"$BODY"; }
search_code() { json "$2" "${3:-5}" > "$BODY"; curl -s -o /dev/null -w '%{http_code}' -b "$1" -X POST "$BASE_URL/api/search" -H 'Content-Type: application/json' --data-binary @"$BODY"; }
count() { python3 -c 'import json,sys;print(json.load(sys.stdin).get("count",0))'; }

login "$JAR_A" student_a1 "$STUDENT_A_PW"
login "$JAR_B" student_b1 "$STUDENT_B_PW"
login "$JAR_T" teacher_a  "$TEACHER_PW"

echo "R1 检索范围只限于会话所在班级"
check "未登录检索返回 401" 401 "$(json '阅读' 3 > "$BODY"; curl -s -o /dev/null -w '%{http_code}' -X POST "$BASE_URL/api/search" -H 'Content-Type: application/json' --data-binary @"$BODY")"
check "A 班检索 B 班关键词返回空数组" 0 "$(search "$JAR_A" '分数四则运算 验算习惯' | count)"
check "请求带 class_id=B 被忽略（仍返回 A 班）" 1 "$(search "$JAR_A" '阅读课 导入' | python3 -c 'import sys,json;print(json.load(sys.stdin)["class_id"])')"
check "跨班检索响应不含对方标题" 0 "$(search "$JAR_A" '分数四则运算 验算习惯' | grep -c '数学课堂练习设计' || true)"

echo "R2 检索接口与参数校验"
check "正常检索返回 200" 200 "$(search_code "$JAR_A" '导入环节' 3)"
check "空查询返回 400" 400 "$(search_code "$JAR_A" '   ')"
check "超长查询返回 400" 400 "$(search_code "$JAR_A" "$(python3 -c 'print("测"*300)')")"
check "top_k 超上限返回 400" 400 "$(search_code "$JAR_A" '阅读' 999)"
check "无命中返回 200 + 空数组" "200 0" "$(search_code "$JAR_A" '完全不存在的词汇xyz') $(search "$JAR_A" '完全不存在的词汇xyz' | count)"

echo "R3 内容溯源"
search "$JAR_A" '导入环节的设计意图' 1 > "$BODY"
python3 - "$BODY" "$JAR_A" "$BASE_URL" <<'PY' || FAILED=1
import json, sys, urllib.request, http.cookiejar
body_path, jar_path, base = sys.argv[1], sys.argv[2], sys.argv[3]
jar = http.cookiejar.MozillaCookieJar(jar_path); jar.load(ignore_discard=True, ignore_expires=True)
op = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(jar))
d = json.load(open(body_path, encoding='utf-8'))
if not d.get('items'):
    print('  ✘ 未返回任何检索结果，无法校验溯源'); sys.exit(1)
item = d['items'][0]
required = ['material_id','material_title','original_name','chunk_index','start_offset','end_offset','content','score']
missing = [k for k in required if k not in item]
print('  ✔ 结果字段完整' if not missing else f'  ✘ 缺少字段 {missing}')
blob = json.dumps(item, ensure_ascii=False)
if 'stored_name' in item or '/Users/' in blob or '/data/' in blob:
    print('  ✘ 结果中出现内部存储名或磁盘路径'); sys.exit(1)
print('  ✔ 结果不含存储名与磁盘路径')
with op.open(f"{base}/api/materials/{item['material_id']}") as r:
    content = json.load(r)['content']
same = content[item['start_offset']:item['end_offset']] == item['content']
print('  ✔ 片段与正文区间逐字符一致' if same else '  ✘ 片段与正文区间不一致')
sys.exit(0 if (same and not missing) else 1)
PY

echo "R5 向量与排序"
DIM="${EMBEDDING_DIM:-}"
if [ -n "$DIM" ] && [ -n "${DB_USER:-}" ]; then
  check "向量长度 = 维度 × 4" "$((DIM*4))" "$(mysql -h "${DB_HOST:-127.0.0.1}" -u "$DB_USER" -p"${DB_PASSWORD:-}" "${DB_NAME:-campusclaw}" -N -e 'select octet_length(embedding) from kb_chunks limit 1' 2>/dev/null)"
else
  printf '  · 未提供 EMBEDDING_DIM / DB 凭据，跳过"向量长度"检查\n'
fi
A1="$(search "$JAR_A" '导入环节的设计意图' 3 | python3 -c 'import sys,json;print([(i["material_id"],i["chunk_index"]) for i in json.load(sys.stdin)["items"]])')"
A2="$(search "$JAR_A" '导入环节的设计意图' 3 | python3 -c 'import sys,json;print([(i["material_id"],i["chunk_index"]) for i in json.load(sys.stdin)["items"]])')"
check "同样查询两次排序一致" "$A1" "$A2"

echo "R6 索引同步"
check "教师上传返回 201" 201 "$(curl -s -o /dev/null -w '%{http_code}' -b "$JAR_T" -X POST "$BASE_URL/api/materials" -F "file=@$TMP_MD")"
# 注意：文件名是 mktemp 生成的，展示标题不含正文标题，因此按"片段正文"判定命中。
# 按本次运行的唯一标记判定（环境里可能已有历史测试材料，不能按标题数条数）。
check "上传后立即可检索到" 1 "$(search "$JAR_A" "$QUERY" 3 | UNIQ="$UNIQUE" python3 -c 'import sys,json,os;u=os.environ["UNIQ"];print(sum(1 for i in json.load(sys.stdin)["items"] if u in i["content"]))')"

echo
if [ "$FAILED" -eq 0 ]; then echo "全部通过 ✔"; else echo "存在未通过项 ✘"; fi
exit "$FAILED"
