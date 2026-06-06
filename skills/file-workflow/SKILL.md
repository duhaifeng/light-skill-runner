---
name: file-workflow
description: 在工作目录内生成测试文件、读回校验、追加日志并输出 JSON 摘要。适合测试文件写入、文件读取、路径安全和多步骤工作流。
---

# File Workflow Skill

当用户要求“测试文件读写”“生成测试产物”“验证工作目录写入能力”“创建一个小报告”时使用本 skill。

## 执行逻辑

1. 如果用户提供主题，把主题作为第一个参数传给脚本；否则使用 `默认主题`。
2. 调用脚本：
   - 路径：`skills/file-workflow/scripts/file_workflow.py`
   - 参数 1：可选，报告主题
3. 脚本会在 `tmp/file-workflow/` 下生成：
   - `report.md`
   - `events.jsonl`
4. 脚本会读回 `report.md` 并计算 SHA256，最终输出 JSON。
5. 最终回答要告诉用户文件路径、摘要哈希和下一步可检查的内容。

## 测试提示词

- “用 file-workflow 生成一个主题为桌面端深度测试的报告”
- “测试文件读写能力”
- “创建一个测试报告并告诉我哈希”
