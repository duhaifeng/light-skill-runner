package runner

import "testing"

// 复现日志 20260606-192610.log 中导致"命令不执行"的坏 JSON（最外层 '}' 缺失）。
func TestParseEmuReply_RecoversTruncatedToolCall(t *testing.T) {
	broken := `{"tool_call": {"name": "run_script", "arguments": {"path": "skills/command-diagnostics/scripts/diagnose.py", "args": ["files"]}}`
	reply, ok := parseEmuReply(broken)
	if !ok {
		t.Fatalf("应能从截断 JSON 中恢复，但解析失败")
	}
	if reply.ToolCall == nil || reply.ToolCall.Name != "run_script" {
		t.Fatalf("期望 tool_call=run_script，得到 %+v", reply.ToolCall)
	}
}

func TestParseEmuReply_Variants(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		wantOK  bool
		wantTC  string // 期望的工具名，空表示非 tool_call
		isFinal bool
	}{
		{"标准 tool_call", `{"tool_call":{"name":"load_skill","arguments":{"name":"x"}}}`, true, "load_skill", false},
		{"final", `{"final":"完成"}`, true, "", true},
		{"代码块包裹", "```json\n{\"tool_call\":{\"name\":\"read_file\",\"arguments\":{}}}\n```", true, "read_file", false},
		{"前置多余文字", `好的：{"tool_call":{"name":"write_file","arguments":{}}}`, true, "write_file", false},
		{"拍平结构", `{"name":"run_script","arguments":{"path":"a"}}`, true, "run_script", false},
		{"尾随逗号+截断", `{"tool_call":{"name":"list_skills","arguments":{},}`, true, "list_skills", false},
		{"纯文本非协议", `你好，今天天气不错`, false, "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			reply, ok := parseEmuReply(c.in)
			if ok != c.wantOK {
				t.Fatalf("ok=%v 期望 %v (输入 %q)", ok, c.wantOK, c.in)
			}
			if !ok {
				return
			}
			if c.isFinal {
				if reply.Final == nil {
					t.Fatalf("期望 final，得到 %+v", reply)
				}
				return
			}
			if reply.ToolCall == nil || reply.ToolCall.Name != c.wantTC {
				t.Fatalf("期望 tool_call=%s，得到 %+v", c.wantTC, reply.ToolCall)
			}
		})
	}
}

func TestLooksLikeProtocol(t *testing.T) {
	if looksLikeProtocol("今天天气如何") {
		t.Fatalf("纯文本不应判为协议")
	}
	if !looksLikeProtocol(`{"tool_call":`) {
		t.Fatalf("含 tool_call 应判为协议")
	}
}
