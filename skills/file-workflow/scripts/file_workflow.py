import hashlib
import json
import sys
from datetime import datetime, timezone
from pathlib import Path

if hasattr(sys.stdout, "reconfigure"):
    sys.stdout.reconfigure(encoding="utf-8")
    sys.stderr.reconfigure(encoding="utf-8")

def main() -> None:
    topic = sys.argv[1] if len(sys.argv) > 1 else "默认主题"
    out_dir = Path("tmp/file-workflow")
    out_dir.mkdir(parents=True, exist_ok=True)

    report = out_dir / "report.md"
    events = out_dir / "events.jsonl"
    now = datetime.now(timezone.utc).isoformat()

    content = "\n".join(
        [
            "# File Workflow Report",
            "",
            f"- topic: {topic}",
            f"- generated_at: {now}",
            "- purpose: verify write/read/hash workflow",
            "",
            "这份文件由 file-workflow skill 生成，用于测试本地文件写入与读回。",
            "",
        ]
    )
    report.write_text(content, encoding="utf-8")

    event = {"time": now, "event": "report_written", "path": str(report), "topic": topic}
    with events.open("a", encoding="utf-8") as f:
        f.write(json.dumps(event, ensure_ascii=False) + "\n")

    read_back = report.read_text(encoding="utf-8")
    digest = hashlib.sha256(read_back.encode("utf-8")).hexdigest()

    print(
        json.dumps(
            {
                "report": str(report),
                "events": str(events),
                "bytes": len(read_back.encode("utf-8")),
                "sha256": digest,
                "preview": read_back.splitlines()[:5],
            },
            ensure_ascii=False,
            indent=2,
        )
    )


if __name__ == "__main__":
    main()
