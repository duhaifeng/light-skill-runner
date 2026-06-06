import json
import os
import platform
import subprocess
import sys
from pathlib import Path

if hasattr(sys.stdout, "reconfigure"):
    sys.stdout.reconfigure(encoding="utf-8")
    sys.stderr.reconfigure(encoding="utf-8")

def run(label: str, cmd: list[str]) -> dict[str, object]:
    try:
        proc = subprocess.run(
            cmd,
            text=True,
            encoding="utf-8",
            errors="replace",
            capture_output=True,
            timeout=8,
        )
        return {
            "label": label,
            "command": cmd,
            "returncode": proc.returncode,
            "stdout": proc.stdout.strip(),
            "stderr": proc.stderr.strip(),
        }
    except FileNotFoundError:
        return {"label": label, "command": cmd, "error": "command not found"}
    except subprocess.TimeoutExpired:
        return {"label": label, "command": cmd, "error": "timeout"}


def file_snapshot() -> dict[str, object]:
    root = Path(".")
    names = sorted(p.name for p in root.iterdir())[:30]
    return {"label": "workspace files", "cwd": str(root.resolve()), "items": names}


def main() -> None:
    mode = sys.argv[1] if len(sys.argv) > 1 else "full"
    results: list[dict[str, object]] = [
        {
            "label": "runtime",
            "python": sys.version.split()[0],
            "platform": platform.platform(),
            "cwd": os.getcwd(),
        }
    ]

    if mode in ("basic", "full"):
        results.append(run("python version", [sys.executable, "--version"]))
        results.append(run("go version", ["go", "version"]))
        results.append(run("node version", ["node", "--version"]))
    if mode in ("files", "full"):
        results.append(file_snapshot())

    print(json.dumps({"mode": mode, "results": results}, ensure_ascii=False, indent=2))


if __name__ == "__main__":
    main()
