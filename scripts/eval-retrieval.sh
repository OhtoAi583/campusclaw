#!/usr/bin/env bash
# 检索质量门禁：跑 eval/retrieval-cases.json，输出命中率与隔离用例结果。
# 用法：BASE_URL=http://localhost:8080 ./scripts/eval-retrieval.sh
set -u
BASE_URL="${BASE_URL:-http://localhost:8080}"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

BASE_URL="$BASE_URL" python3 - <<'PY'
import json, os, sys, urllib.request, http.cookiejar

BASE = os.environ.get("BASE_URL", "http://localhost:8080")
CASES = json.load(open("eval/retrieval-cases.json", encoding="utf-8"))

CREDENTIALS = {
    "student_a1": os.environ.get("STUDENT_A_PW", ""),
    "student_b1": os.environ.get("STUDENT_B_PW", ""),
    "teacher_a": os.environ.get("TEACHER_PW", ""),
}

def opener_for(user):
    jar = http.cookiejar.CookieJar()
    op = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(jar))
    body = json.dumps({"username": user, "password": CREDENTIALS[user]}).encode()
    req = urllib.request.Request(BASE + "/api/login", data=body, headers={"Content-Type": "application/json"})
    op.open(req).read()
    return op

def search(op, query):
    body = json.dumps({"query": query, "top_k": 5}).encode()
    req = urllib.request.Request(BASE + "/api/search", data=body, headers={"Content-Type": "application/json"})
    return json.loads(op.open(req).read().decode())

missing = [u for u, p in CREDENTIALS.items() if not p]
if missing:
    print("缺少预置口令环境变量（STUDENT_A_PW / STUDENT_B_PW / TEACHER_PW）:", ", ".join(missing))
    sys.exit(2)

sessions = {}
hits = total_hit = 0
isolation_failed = []

for case in CASES["cases"]:
    user = case["as"]
    sessions.setdefault(user, opener_for(user))
    try:
        data = search(sessions[user], case["query"])
    except Exception as e:  # noqa: BLE001
        print(f"  ✘ {case['name']}：请求失败 {e}")
        if case["kind"] == "isolation":
            isolation_failed.append(case["name"])
        continue

    items = data["items"]
    if case["kind"] == "isolation":
        leaked = [i for i in items if case["forbidden_material_title_contains"] in i["material_title"]]
        if leaked:
            isolation_failed.append(case["name"])
            print(f"  ✘ {case['name']}：出现跨班结果 {leaked[0]['material_title']}")
        else:
            print(f"  ✔ {case['name']}：未返回跨班内容（结果数 {len(items)}）")
    else:
        total_hit += 1
        matched = [
            i for i in items
            if case["expect_material_title_contains"] in i["material_title"]
            and case["expect_content_contains"] in i["content"]
        ]
        if matched:
            hits += 1
            print(f"  ✔ {case['name']}：命中 {matched[0]['material_title']}（score={matched[0]['score']:.3f}）")
        else:
            print(f"  ✘ {case['name']}：未命中期望来源（返回 {len(items)} 条）")

rate = hits / total_hit if total_hit else 0
print()
print(f"命中率：{hits}/{total_hit} = {rate:.0%}（阈值 {CASES['threshold']:.0%}）")
print(f"隔离用例：{'全部通过 ✔' if not isolation_failed else '失败 ' + str(isolation_failed) + ' ✘'}")

ok = rate >= CASES["threshold"] and not isolation_failed
print("结论：", "评测通过 ✔" if ok else "评测未通过 ✘")
sys.exit(0 if ok else 1)
PY
