#!/usr/bin/env bash
# Полная проверка на Docker-хосте с удалением только созданных ею объектов.
set -euo pipefail
image="symbol-web-dockerize-audit:${RANDOM}-$$"
container="symbol-web-audit-${RANDOM}-$$"
workspace="$(mktemp -d)"
cleanup() {
  docker container rm --force "$container" >/dev/null 2>&1 || true
  docker image rm "$image" >/dev/null 2>&1 || true
  rm -rf "$workspace"
}
trap cleanup EXIT
docker image build -f Dockerfile -t "$image" .
docker container run --detach --name "$container" \
  --label io.tomorrow.project=symbol-web-dockerize \
  --read-only --cap-drop ALL --security-opt no-new-privileges \
  --pids-limit 64 --memory 128m --cpus 1 -p 127.0.0.1::8080 "$image"
binding="$(docker container port "$container" 8080/tcp)"
base="http://${binding}"
for attempt in $(seq 1 60); do
  if curl --fail --silent "$base/healthz" > "$workspace/health.json"; then break; fi
  sleep 1
done
curl --fail --silent "$base/" > "$workspace/index.html"
grep -q 'id="text-input"' "$workspace/index.html"
for banner in standard shadow thinkertoy; do
  curl --fail --silent --data-urlencode 'text=Hello, Docker!' --data-urlencode "banner=$banner" "$base/symbol-art" > "$workspace/$banner.html"
  grep -q 'id="art-output"' "$workspace/$banner.html"
done
curl --fail --silent "$base/static/css/style.css" > /dev/null
curl --fail --silent "$base/static/js/script.js" > /dev/null
curl --fail --silent --header 'Content-Type: application/json' --data '{"text":"Hello"}' "$base/api/suggest" > "$workspace/suggest.json"
docker container exec "$container" /bin/bash -c 'test -x /app/server && test -d /app/static && test -d /app/templates && test -r /app/standard.txt && test "$(id -u)" != 0'
docker container inspect --format '{{index .Config.Labels "io.tomorrow.project"}}' "$container" | grep -qx symbol-web-dockerize
docker image inspect --format '{{index .Config.Labels "org.opencontainers.image.title"}}' "$image" | grep -qx symbol-web-dockerize
go build -o "$workspace/health-monitor" ./cmd/health-monitor
"$workspace/health-monitor" --lint Dockerfile
"$workspace/health-monitor" --json "$container" > "$workspace/monitor.json"
python3 - "$workspace/monitor.json" <<'PY'
import json, sys
report = json.load(open(sys.argv[1], encoding="utf-8"))
assert report["status"] == "running"
assert report["health_score"] == sum(report["scores"].values())
assert 0 <= report["health_score"] <= 100
assert set(report["metrics"]) == {"uptime_seconds", "memory_percent", "cpu_percent", "restart_count"}
print(json.dumps(report, ensure_ascii=False, indent=2))
PY
docker container stop --time 5 "$container"
if "$workspace/health-monitor" --json "$container" > /dev/null 2>&1; then
  echo 'Ошибка: остановленный контейнер получил оценку.' >&2
  exit 1
fi
echo 'Проверки контейнера завершены успешно.'
