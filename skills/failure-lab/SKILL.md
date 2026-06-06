---
name: failure-lab
description: 触发可控成功、警告、非零退出和短暂延迟场景，用来测试错误处理、stderr、日志记录和运行透视。适合深度测试排障链路。
---

# Failure Lab Skill

当用户要求“测试失败场景”“验证错误日志”“模拟脚本异常”“检查 stderr 是否记录”时使用本 skill。

## 模式

调用脚本：

- 路径：`skills/failure-lab/scripts/failure_lab.py`
- 参数 1：
  - `ok`：正常成功。
  - `warn`：向 stderr 输出警告，但退出码为 0。
  - `fail`：向 stderr 输出错误，并以非零退出码结束。
  - `slow`：短暂等待后成功，用于观察耗时。

如果用户没有指定模式，默认使用 `warn`。

## 回答要求

1. 明确说明触发了哪个模式。
2. 如果工具调用失败，要把错误原因解释成“预期的测试失败”，不要把它误判为系统不可用。
3. 引导用户查看 `logs/<trace_id>.log` 和 `logs/runs.log`。

## 测试提示词

- “用 failure-lab 跑 warn 模式”
- “模拟一次脚本失败，看看日志怎么记录”
- “用 slow 模式测试耗时显示”
