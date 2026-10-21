#!/usr/bin/env bash
# Runs on the existing server. No env sourcing, service restart or messaging.
set -euo pipefail
operation=${1:-check}
slot=${2:?unique workflow run-attempt required}
case "$operation" in check|apply|withdraw|model-preview) ;; *) exit 2 ;; esac
[[ "$slot" =~ ^[0-9]+-[0-9]+$ ]] || exit 2
db=/opt/thg-scraper/data/scraper.db

# Serialize this immutable release, including checks and withdrawal.
exec 9>/run/lock/thg-company-knowledge-release.lock
flock -n 9 || { echo 'release_lock=busy'; exit 1; }

# Prove this is the file both running services actually use. Do not print
# process environments, org IDs, connection strings or production records.
python3 - "$db" <<'PY'
import subprocess
import sys
from pathlib import Path

try:
    expected = Path(sys.argv[1]).resolve(strict=True)
    for service in ('thg-scraper', 'thg-worker'):
        pid = subprocess.check_output(
            ['systemctl', 'show', service, '--property=MainPID', '--value'],
            text=True, stderr=subprocess.DEVNULL).strip()
        if not pid.isdigit() or int(pid) <= 0:
            raise ValueError('inactive')
        entries = Path('/proc/' + pid + '/environ').read_bytes().split(b'\0')
        env = dict(entry.split(b'=', 1) for entry in entries if b'=' in entry)
        raw = env.get(b'DB_PATH', b'data/scraper.db').decode('utf-8')
        actual = (Path('/proc/' + pid + '/cwd').resolve() / raw).resolve(strict=True)
        if actual != expected:
            raise ValueError('different store')
except (OSError, ValueError, subprocess.SubprocessError):
    sys.exit('runtime_database=unverified')
print('runtime_database=verified')
PY

if [[ "$operation" = apply || "$operation" = withdraw ]]; then
  backup_dir=/opt/thg-scraper/backups/company-knowledge
  install -d -m 0700 "$backup_dir"
  backup="$backup_dir/release-$slot.db"
  [[ ! -e "$backup" ]]
  (umask 077; sqlite3 "$db" ".backup '$backup'")
  [[ "$(sqlite3 -readonly "$backup" 'PRAGMA quick_check;')" = ok ]]
  echo 'release_backup=verified'
fi

if [[ "$operation" = model-preview ]]; then
  checked=$(./company-knowledge-release -db "$db" -operation check -manifest review.json)
  printf '%s\n' "$checked"
  printf '%s' "$checked" | python3 -c 'import json,sys; sys.exit(0 if json.load(sys.stdin)["status"] == "loaded_and_retrievable" else 1)'
  for post in \
    'Mình cần kho tại Mỹ để lưu hàng, xử lý đơn và hàng hoàn. Bên bạn hỗ trợ không?' \
    'I am looking for a shipping agent from China to the US for 200 kg monthly. Can you help?' \
    'Mình muốn tìm nguồn hàng 1688, quy trình bắt đầu thế nào?' \
    'Welcome to our Amazon community. Here are this week’s announcements.'; do
    ./company-knowledge-preview -pack public-preview -live-pipeline -review-draft -public-facts-only \
      -env-file /etc/thg-scraper/env -query "$post" -author 'Synthetic review'
  done
  exit 0
fi

./company-knowledge-release -db "$db" -operation "$operation" -manifest review.json
