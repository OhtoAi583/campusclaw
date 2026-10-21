#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""采集验收证据：真实调用平台接口，抓取每次 LLM 调用的完整请求/响应。

输出 evidence.json，供报告生成脚本使用。
"""
import json
import os
import sys
import urllib.request
import http.cookiejar

BASE = os.environ.get("BASE_URL", "http://127.0.0.1:8080")
USER = os.environ.get("TEACHER_USER", "teacher_a")
PASS = os.environ.get("TEACHER_PW", "teacher-a-demo-2026")


class Client:
    def __init__(self):
        jar = http.cookiejar.CookieJar()
        self.op = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(jar))

    def req(self, method, path, body=None):
        data = json.dumps(body).encode() if body is not None else None
        r = urllib.request.Request(BASE + path, data=data, method=method,
                                   headers={"Content-Type": "application/json"})
        with self.op.open(r) as resp:
            raw = resp.read().decode()
            return json.loads(raw) if raw else {}

    def get(self, path):
        return self.req("GET", path)

    def post(self, path, body):
        return self.req("POST", path, body)


def describe_call(call):
    req, resp = call["request"], call["response"]
    msgs = req.get("messages", [])
    choice = (resp.get("choices") or [{}])[0]
    return {
        "id": call["id"],
        "round_index": call["round_index"],
        "model": req.get("model"),
        "temperature": req.get("temperature"),
        "max_tokens": req.get("max_tokens"),
        "stream": req.get("stream"),
        "request_fields": sorted(req.keys()),
        "messages": [{"role": m["role"], "chars": len(m.get("content") or "")} for m in msgs],
        "system": msgs[0]["content"] if msgs else "",
        "user": msgs[1]["content"] if len(msgs) > 1 else "",
        "tools": req.get("tools") or [],
        "response_content": (choice.get("message") or {}).get("content") or "",
        "response_tool_calls": (choice.get("message") or {}).get("tool_calls") or [],
        "finish_reason": choice.get("finish_reason"),
        "usage": resp.get("usage") or {},
        "latency_ms": call.get("latency_ms"),
        "kb_chunks": call.get("kb_chunks"),
        "kb_chars": call.get("kb_chars"),
        "skill_chars": call.get("skill_chars"),
        "tool_rounds": call.get("tool_rounds"),
    }


def main():
    c = Client()
    c.post("/api/login", {"username": USER, "password": PASS})

    apps = c.get("/api/apps")["items"]
    templates = c.get("/api/templates")["items"]
    skills = c.get("/api/skills")["items"]
    mcp = c.get("/api/mcp/servers")["items"]
    mcp_tools = c.get("/api/mcp/servers/demo-tools/tools")["tools"]

    out = {"base": BASE, "apps": apps, "templates": templates, "skills": skills,
           "mcp_servers": mcp, "mcp_tools": mcp_tools, "scenarios": {}}

    # 场景 1：单应用（知识库 + 1 个技能）
    r1 = c.post("/api/chat", {"app_id": "kb-qa", "question": "导入环节为什么要用情境图？"})
    calls1 = [describe_call(c.get("/api/llm-calls/%d" % cid)) for cid in r1["meta"]["llm_call_ids"]]
    out["scenarios"]["qa"] = {"result": {k: r1[k] for k in ("answer", "citations", "meta")}, "calls": calls1}

    # 场景 2：两个技能 + 教案模板
    r2 = c.post("/api/chat", {"app_id": "lesson-plan", "question": "请设计一节关于人物形象分析的阅读课。"})
    calls2 = [describe_call(c.get("/api/llm-calls/%d" % cid)) for cid in r2["meta"]["llm_call_ids"]]
    out["scenarios"]["lesson_plan"] = {"result": {k: r2[k] for k in ("answer", "citations", "meta")}, "calls": calls2}

    # 场景 3：MCP 工具调用
    r3 = c.post("/api/chat", {"app_id": "data-analysis",
                              "question": "请统计本班课堂观察记录的字数与平均句长，并据此给出学情分析建议。"})
    calls3 = [describe_call(c.get("/api/llm-calls/%d" % cid)) for cid in r3["meta"]["llm_call_ids"]]
    out["scenarios"]["mcp"] = {"result": {k: r3[k] for k in ("answer", "citations", "meta")}, "calls": calls3}

    with open(os.path.join(os.path.dirname(os.path.abspath(__file__)), "evidence.json"), "w",
              encoding="utf-8") as f:
        json.dump(out, f, ensure_ascii=False, indent=2)

    qa = out["scenarios"]["qa"]
    print("应用数:", len(apps), "模板数:", len(templates), "技能数:", len(skills), "MCP:", len(mcp))
    print("Q1 请求字段:", qa["calls"][0]["request_fields"])
    print("Q2 知识库: 总切片=%s 传入切片=%s 传入字符=%s" %
          (qa["result"]["meta"]["kb_total_chunks"], qa["result"]["meta"]["kb_chunks"], qa["result"]["meta"]["kb_chars"]))
    print("Q3 技能:", qa["result"]["meta"]["skill_ids"], "注入字符:", qa["result"]["meta"]["skill_chars"])
    print("Q5 MCP: tools=%d 轮次=%d 调用数=%d" %
          (len(out["scenarios"]["mcp"]["calls"][0]["tools"]), out["scenarios"]["mcp"]["result"]["meta"]["tool_rounds"],
           len(out["scenarios"]["mcp"]["calls"])))
    print("已写入 evidence.json")


if __name__ == "__main__":
    main()
