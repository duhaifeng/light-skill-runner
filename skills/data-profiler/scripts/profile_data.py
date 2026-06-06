import csv
import json
import statistics
import sys
from pathlib import Path

if hasattr(sys.stdout, "reconfigure"):
    sys.stdout.reconfigure(encoding="utf-8")
    sys.stderr.reconfigure(encoding="utf-8")

SAMPLE = """name,team,score,latency_ms
Alice,alpha,91,120
Bob,beta,83,145
Carol,alpha,95,110
Dave,beta,,160
"""


def parse_input(raw: str) -> list[dict[str, str]]:
    text = raw.strip() or SAMPLE
    if text.startswith("["):
        items = json.loads(text)
        return [{str(k): "" if v is None else str(v) for k, v in row.items()} for row in items]
    return list(csv.DictReader(text.splitlines()))


def numeric(values: list[str]) -> list[float]:
    out: list[float] = []
    for value in values:
        if value == "":
            continue
        try:
            out.append(float(value))
        except ValueError:
            pass
    return out


def build_report(rows: list[dict[str, str]]) -> str:
    fields = list(rows[0].keys()) if rows else []
    lines = [
        "# Data Profile Report",
        "",
        f"- rows: {len(rows)}",
        f"- fields: {', '.join(fields) if fields else '(none)'}",
        "",
        "## Columns",
    ]
    for field in fields:
        values = [row.get(field, "") for row in rows]
        missing = sum(1 for value in values if value == "")
        nums = numeric(values)
        lines.append(f"- {field}: missing={missing}")
        if nums:
            lines.append(
                f"  - numeric min={min(nums):.2f}, max={max(nums):.2f}, "
                f"mean={statistics.mean(nums):.2f}"
            )
    return "\n".join(lines)


def main() -> None:
    args = sys.argv[1:]
    should_write = "write-report" in args
    data_args = [arg for arg in args if arg != "write-report"]
    raw = data_args[0] if data_args else SAMPLE
    rows = parse_input(raw)
    report = build_report(rows)
    if should_write:
        out = Path("tmp/data-profiler-report.md")
        out.parent.mkdir(parents=True, exist_ok=True)
        out.write_text(report, encoding="utf-8")
        print(f"报告已写入: {out}")
    print(report)


if __name__ == "__main__":
    main()
