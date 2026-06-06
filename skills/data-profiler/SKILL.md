---
name: data-profiler
description: 分析一小段 CSV 或 JSON 数据，输出字段、行数、数值统计和缺失值摘要，并可写入 Markdown 报告。适合测试参数传递、文本解析、文件写入和结构化结果。
---

# Data Profiler Skill

当用户要求“分析数据”“生成数据摘要”“统计 CSV/JSON”“写一份数据报告”时使用本 skill。

## 执行策略

1. 判断用户输入的数据格式：
   - 如果用户明确提供 CSV 文本，把内容作为第一个参数传给脚本。
   - 如果用户明确提供 JSON 数组，把内容作为第一个参数传给脚本。
   - 如果用户只要求演示或测试，不提供数据，则让脚本使用内置示例数据。
2. 如果用户要求保存报告，第二个参数传 `write-report`，脚本会写入 `tmp/data-profiler-report.md`。
3. 调用脚本：
   - 路径：`skills/data-profiler/scripts/profile_data.py`
   - 参数 1：可选，CSV 或 JSON 文本
   - 参数 2：可选，`write-report`
4. 把脚本输出作为最终回答的依据，说明数据规模、字段、缺失值和数值统计。

## 测试提示词

- “用 data-profiler 分析一份示例数据”
- “分析这些 CSV，并写报告：name,age,score\nAlice,30,88\nBob,,92”
- “统计这个 JSON 数组：[{\"name\":\"A\",\"score\":10},{\"name\":\"B\",\"score\":15}]”
