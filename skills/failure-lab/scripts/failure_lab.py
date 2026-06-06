import json
import sys
import time

if hasattr(sys.stdout, "reconfigure"):
    sys.stdout.reconfigure(encoding="utf-8")
    sys.stderr.reconfigure(encoding="utf-8")

def emit(mode: str, status: str) -> None:
    print(json.dumps({"mode": mode, "status": status}, ensure_ascii=False))


def main() -> None:
    mode = sys.argv[1] if len(sys.argv) > 1 else "warn"
    if mode == "ok":
        emit(mode, "success")
        return
    if mode == "warn":
        print("这是 stderr 警告：用于验证日志是否记录警告。", file=sys.stderr)
        emit(mode, "success_with_warning")
        return
    if mode == "slow":
        print("开始 slow 模式，等待 2 秒。")
        time.sleep(2)
        emit(mode, "success_after_delay")
        return
    if mode == "fail":
        print("这是预期内的失败：failure-lab fail 模式。", file=sys.stderr)
        emit(mode, "failed_as_designed")
        raise SystemExit(7)
    print(f"未知模式: {mode}", file=sys.stderr)
    raise SystemExit(2)


if __name__ == "__main__":
    main()
