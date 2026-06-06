---
name: hello-world
description: 一个最小示例 skill，用于演示引擎如何加载并执行 skill。当用户想要打招呼、测试引擎是否正常工作时使用。
---

# Hello World Skill

这是一个用于验证 `light-skill-runner` 是否正常工作的最小 skill。

## 执行步骤

1. 友好地向用户问好。
2. 如果用户提供了名字，则在问候中带上名字；否则称呼为"朋友"。
3. 告诉用户当前 skill 引擎工作正常。

## 可选：运行脚本

本 skill 目录下有一个 `scripts/greet.py`，可用 `run_script` 工具执行它来生成问候语：

- 路径：`skills/hello-world/scripts/greet.py`
- 参数：第一个参数为要问候的名字（可选）

执行后把脚本输出作为问候内容回复给用户。
