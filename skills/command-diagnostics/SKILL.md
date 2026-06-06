---
name: command-diagnostics
description: 执行一组安全的系统诊断命令，收集 Go/Python/Node 版本、当前目录、文件列表等信息。适合测试命令执行、超时、stdout/stderr 合并和跨平台行为。
---

# Command Diagnostics Skill

当用户要求“检查环境”“诊断命令执行”“看看当前项目状态”“测试命令工具”时使用本 skill。

## 安全边界

本 skill 只允许执行脚本内置的白名单诊断项，不执行用户输入的任意 shell 命令，不做删除、修改、网络请求或危险操作。

## 执行步骤

1. 选择诊断模式：
   - `basic`：当前目录、Python 版本、Go 版本。
   - `files`：列出工作目录下的关键文件和目录。
   - `full`：执行全部安全诊断。
2. 调用脚本：
   - 路径：`skills/command-diagnostics/scripts/diagnose.py`
   - 参数 1：`basic`、`files` 或 `full`，默认 `full`
3. 总结每个命令的退出码和输出。如果某个命令不存在，要说明是环境缺失而不是系统失败。

## 测试提示词

- “运行 command-diagnostics 做一次 full 诊断”
- “检查当前环境里 Go 和 Python 是否可用”
- “测试一下命令执行能力，只列出文件”
