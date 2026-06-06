import sys


def main() -> None:
    name = sys.argv[1] if len(sys.argv) > 1 else "朋友"
    print(f"你好，{name}！light-skill-runner 工作正常。")


if __name__ == "__main__":
    main()
