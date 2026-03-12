#!/usr/bin/env bash
# dns-sync.sh — Reconcile zone YAML configs to Cloudflare DNS
#
# Usage:
#   dns-sync.sh [zone-file.yaml]          # Dry-run (default)
#   dns-sync.sh --apply [zone-file.yaml]  # Apply changes
#   dns-sync.sh --all                     # Dry-run all zones
#   dns-sync.sh --apply --all             # Apply all zones
#
# Requires:
#   CF_API_EMAIL and CF_API_KEY env vars (Cloudflare Global API Key)
#   python3, curl, yq (or pyyaml)
#
# The script reads zone YAML files from zones/ directory and compares
# them against live Cloudflare state. It reports differences and
# optionally applies changes (create/update/delete).

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ZONES_DIR="${SCRIPT_DIR}/../zones"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

APPLY=false
ALL=false
ZONE_FILE=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --apply) APPLY=true; shift ;;
    --all)   ALL=true; shift ;;
    *)       ZONE_FILE="$1"; shift ;;
  esac
done

if [[ -z "${CF_API_EMAIL:-}" || -z "${CF_API_KEY:-}" ]]; then
  echo -e "${RED}Error: CF_API_EMAIL and CF_API_KEY must be set${NC}"
  echo "  export CF_API_EMAIL=you@example.com"
  echo "  export CF_API_KEY=your-global-api-key"
  exit 1
fi

cf_api() {
  local method="$1" path="$2"
  shift 2
  curl -s -X "$method" \
    "https://api.cloudflare.com/client/v4${path}" \
    -H "X-Auth-Email: ${CF_API_EMAIL}" \
    -H "X-Auth-Key: ${CF_API_KEY}" \
    -H "Content-Type: application/json" \
    "$@"
}

sync_zone() {
  local yaml_file="$1"
  local zone_name
  zone_name="$(basename "$yaml_file" .yaml | tr '-' '.')"

  echo -e "${BLUE}=== Syncing ${zone_name} ===${NC}"

  python3 << PYEOF
import json, yaml, sys

with open("${yaml_file}") as f:
    config = yaml.safe_load(f)

zone = config['zone']
zone_id = config['zone_id']
desired = config.get('records', [])

# Fetch live records
import subprocess
result = subprocess.run([
    'curl', '-s', '-X', 'GET',
    f'https://api.cloudflare.com/client/v4/zones/{zone_id}/dns_records?per_page=500',
    '-H', 'X-Auth-Email: ${CF_API_EMAIL}',
    '-H', 'X-Auth-Key: ${CF_API_KEY}',
    '-H', 'Content-Type: application/json'
], capture_output=True, text=True)
live_data = json.loads(result.stdout)

if not live_data.get('success'):
    print(f"  ERROR fetching zone {zone}: {live_data.get('errors')}")
    sys.exit(1)

live_records = live_data['result']

# Normalize desired records
def normalize_name(name, zone):
    name = str(name)
    if name == '@':
        return zone
    if not name.endswith(zone):
        return f'{name}.{zone}'
    return name

def normalize_txt_content(content):
    """Strip outer quotes from TXT content for comparison"""
    c = str(content)
    if c.startswith('"') and c.endswith('"'):
        c = c[1:-1]
    return c

for r in desired:
    r['name'] = normalize_name(r['name'], zone)
    r.setdefault('proxied', False)
    r.setdefault('ttl', 1)

# Build lookup keys
def record_key(r):
    content = r.get('content', '')
    if r['type'] == 'TXT':
        content = normalize_txt_content(content)
    return (r['type'], r['name'], content)

live_by_key = {}
for r in live_records:
    key = record_key(r)
    live_by_key[key] = r

desired_by_key = {}
for r in desired:
    key = record_key(r)
    desired_by_key[key] = r

# Compare
to_create = []
to_update = []
to_delete = []

for key, d in desired_by_key.items():
    if key in live_by_key:
        live = live_by_key[key]
        # Check if proxied or ttl differs
        if live.get('proxied') != d.get('proxied') or (d.get('ttl', 1) != 1 and live.get('ttl') != d.get('ttl')):
            to_update.append((d, live['id']))
    else:
        to_create.append(d)

for key, live in live_by_key.items():
    if key not in desired_by_key:
        # Skip ACME challenge records (managed by Traefik)
        if '_acme-challenge' in live['name']:
            continue
        to_delete.append(live)

apply = "${APPLY}" == "true"

# Report
if not to_create and not to_update and not to_delete:
    print(f"  \033[0;32m✓ {zone} is in sync ({len(live_records)} records)\033[0m")
    sys.exit(0)

if to_create:
    print(f"  \033[0;32mCREATE ({len(to_create)}):\033[0m")
    for r in to_create:
        print(f"    + {r['type']:6s} {r['name']:50s} -> {r.get('content','')}")

if to_update:
    print(f"  \033[1;33mUPDATE ({len(to_update)}):\033[0m")
    for r, rid in to_update:
        print(f"    ~ {r['type']:6s} {r['name']:50s} -> {r.get('content','')}")

if to_delete:
    print(f"  \033[0;31mDELETE ({len(to_delete)}):\033[0m")
    for r in to_delete:
        print(f"    - {r['type']:6s} {r['name']:50s} -> {r.get('content','')}")

if not apply:
    total = len(to_create) + len(to_update) + len(to_delete)
    print(f"  \033[1;33mDry run: {total} changes pending. Use --apply to execute.\033[0m")
    sys.exit(0)

# Apply changes
errors = 0

for r in to_create:
    body = json.dumps({
        'type': r['type'],
        'name': r['name'],
        'content': r.get('content', ''),
        'proxied': r.get('proxied', False),
        'ttl': r.get('ttl', 1),
    })
    result = subprocess.run([
        'curl', '-s', '-X', 'POST',
        f'https://api.cloudflare.com/client/v4/zones/{zone_id}/dns_records',
        '-H', 'X-Auth-Email: ${CF_API_EMAIL}',
        '-H', 'X-Auth-Key: ${CF_API_KEY}',
        '-H', 'Content-Type: application/json',
        '-d', body
    ], capture_output=True, text=True)
    resp = json.loads(result.stdout)
    if resp.get('success'):
        print(f"    ✓ Created {r['type']} {r['name']}")
    else:
        errors += 1
        print(f"    ✗ Failed to create {r['name']}: {resp.get('errors')}")

for r, rid in to_update:
    body = json.dumps({
        'type': r['type'],
        'name': r['name'],
        'content': r.get('content', ''),
        'proxied': r.get('proxied', False),
        'ttl': r.get('ttl', 1),
    })
    result = subprocess.run([
        'curl', '-s', '-X', 'PUT',
        f'https://api.cloudflare.com/client/v4/zones/{zone_id}/dns_records/{rid}',
        '-H', 'X-Auth-Email: ${CF_API_EMAIL}',
        '-H', 'X-Auth-Key: ${CF_API_KEY}',
        '-H', 'Content-Type: application/json',
        '-d', body
    ], capture_output=True, text=True)
    resp = json.loads(result.stdout)
    if resp.get('success'):
        print(f"    ✓ Updated {r['type']} {r['name']}")
    else:
        errors += 1
        print(f"    ✗ Failed to update {r['name']}: {resp.get('errors')}")

for r in to_delete:
    result = subprocess.run([
        'curl', '-s', '-X', 'DELETE',
        f'https://api.cloudflare.com/client/v4/zones/{zone_id}/dns_records/{r["id"]}',
        '-H', 'X-Auth-Email: ${CF_API_EMAIL}',
        '-H', 'X-Auth-Key: ${CF_API_KEY}',
        '-H', 'Content-Type: application/json'
    ], capture_output=True, text=True)
    resp = json.loads(result.stdout)
    if resp.get('success'):
        print(f"    ✓ Deleted {r['type']} {r['name']}")
    else:
        errors += 1
        print(f"    ✗ Failed to delete {r['name']}: {resp.get('errors')}")

applied = len(to_create) + len(to_update) + len(to_delete) - errors
print(f"  Applied {applied} changes ({errors} errors)")
PYEOF
}

if [[ "$ALL" == true ]]; then
  for f in "${ZONES_DIR}"/*.yaml; do
    sync_zone "$f"
    echo ""
  done
elif [[ -n "$ZONE_FILE" ]]; then
  if [[ ! -f "$ZONE_FILE" ]]; then
    # Try zones/ directory
    ZONE_FILE="${ZONES_DIR}/${ZONE_FILE}"
  fi
  if [[ ! -f "$ZONE_FILE" ]]; then
    echo -e "${RED}Zone file not found: $ZONE_FILE${NC}"
    exit 1
  fi
  sync_zone "$ZONE_FILE"
else
  echo "Usage: dns-sync.sh [--apply] [--all | zone-file.yaml]"
  echo ""
  echo "Available zones:"
  for f in "${ZONES_DIR}"/*.yaml; do
    echo "  $(basename "$f")"
  done
  exit 0
fi
