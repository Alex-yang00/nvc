#!/usr/bin/env bash
# Headless smoke test: nvc <harness> runs a fixed edit-and-test task in a temp repo.
# Usage: scripts/smoke/run.sh claude|codex [nvc flags...]
set -uo pipefail
H=$1; shift
NVC=${NVC:-$(cd "$(dirname "$0")/../.." && pwd)/bin/nvc}
DIR=$(mktemp -d /tmp/nvc-smoke-$H-XXXX)
cd "$DIR"
git init -q
cat > README.md <<'MD'
# mathx
Small math helpers in `mathx.py`. Tests live in `test_mathx.py` and run with `python3 -m unittest`.
MD
printf 'def add(a, b):\n    return a + b\n' > mathx.py
printf 'import unittest\nfrom mathx import add\n\nclass T(unittest.TestCase):\n    def test_add(self):\n        self.assertEqual(add(1, 2), 3)\n\nif __name__ == "__main__":\n    unittest.main()\n' > test_mathx.py
git add -A && git -c user.email=s@s -c user.name=s commit -qm init

TASK='Read README.md. Add a function clamp(x, lo, hi) to mathx.py, add unit tests for it to test_mathx.py, then run the tests with the shell and report the result.'
start=$(date +%s)
case $H in
  claude) "$NVC" claude "$@" -p "$TASK" --output-format json --permission-mode acceptEdits --allowedTools "Bash(python3:*)" < /dev/null > out.json 2> err.txt ;;
  codex)  "$NVC" codex "$@" exec --sandbox workspace-write --skip-git-repo-check "$TASK" < /dev/null > out.txt 2> err.txt ;;
esac
rc=$?
secs=$(( $(date +%s) - start ))
python3 -m unittest -q 2> verify.txt; vrc=$?
grep -q "def clamp" mathx.py && grep -q clamp test_mathx.py; frc=$?
echo "harness=$H exit=$rc secs=$secs files_ok=$([ $frc = 0 ] && echo yes || echo no) tests_pass=$([ $vrc = 0 ] && echo yes || echo no) dir=$DIR"
