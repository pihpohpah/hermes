#!/usr/bin/env bash
# shellcheck disable=SC2029 # The validated DEPLOY_PATH intentionally expands on the client.
set -Eeuo pipefail

ROOT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

ENV_FILE="${DEPLOY_ENV_FILE:-deploy.env}"
if [[ ! -f "$ENV_FILE" ]]; then
    printf 'Missing %s. Copy deploy.env.example and fill in the deployment settings.\n' "$ENV_FILE" >&2
    exit 1
fi

set -a
# shellcheck disable=SC1090
source "$ENV_FILE"
set +a

: "${HERMES_IMAGE:?Set HERMES_IMAGE in deploy.env}"
: "${DEPLOY_HOST:?Set DEPLOY_HOST in deploy.env}"
: "${DEPLOY_USER:?Set DEPLOY_USER in deploy.env}"
: "${DEPLOY_PATH:?Set DEPLOY_PATH in deploy.env}"

HERMES_TAG="${HERMES_TAG:-$(git rev-parse --short HEAD)}"
BUILD_PLATFORM="${BUILD_PLATFORM:-linux/amd64}"
DEPLOY_SSH_PORT="${DEPLOY_SSH_PORT:-22}"
DEPLOY_HEALTH_TIMEOUT="${DEPLOY_HEALTH_TIMEOUT:-60}"
SKIP_BUILD="${SKIP_BUILD:-0}"
POSTGRES_USER="${POSTGRES_USER:-hermes}"
POSTGRES_DB="${POSTGRES_DB:-hermes}"

if [[ ! "$HERMES_IMAGE" =~ ^[a-z0-9][a-z0-9._:/-]*$ ]]; then
    printf 'Invalid HERMES_IMAGE: %s\n' "$HERMES_IMAGE" >&2
    exit 1
fi
if [[ ! "$HERMES_TAG" =~ ^[[:alnum:]_.-]+$ ]]; then
    printf 'Invalid HERMES_TAG: %s\n' "$HERMES_TAG" >&2
    exit 1
fi
if [[ ! "$DEPLOY_HOST" =~ ^[[:alnum:]_.-]+$ ]]; then
    printf 'DEPLOY_HOST must be a hostname, IPv4 address, or SSH config alias.\n' >&2
    exit 1
fi
if [[ ! "$DEPLOY_USER" =~ ^[a-z_][a-z0-9_.-]*$ ]]; then
    printf 'Invalid DEPLOY_USER: %s\n' "$DEPLOY_USER" >&2
    exit 1
fi
if [[ ! "$DEPLOY_PATH" =~ ^/[[:alnum:]_./-]+$ ]]; then
    printf 'DEPLOY_PATH must be an absolute path without spaces or shell characters.\n' >&2
    exit 1
fi
if [[ ! "$DEPLOY_HEALTH_TIMEOUT" =~ ^[1-9][0-9]*$ ]]; then
    printf 'DEPLOY_HEALTH_TIMEOUT must be a positive integer.\n' >&2
    exit 1
fi
if [[ ! "$DEPLOY_SSH_PORT" =~ ^[1-9][0-9]*$ ]]; then
    printf 'DEPLOY_SSH_PORT must be a positive integer.\n' >&2
    exit 1
fi

for command_name in awk docker git ssh scp; do
    if ! command -v "$command_name" >/dev/null 2>&1; then
        printf 'Required command is not installed: %s\n' "$command_name" >&2
        exit 1
    fi
done

SSH_TARGET="${DEPLOY_USER}@${DEPLOY_HOST}"
SSH_OPTIONS=(-p "$DEPLOY_SSH_PORT")
SCP_OPTIONS=(-P "$DEPLOY_SSH_PORT")

# Каталог под офлайн-карты (PMTiles) монтируется в контейнер из DEPLOY_PATH.
ssh "${SSH_OPTIONS[@]}" "$SSH_TARGET" "install -d -m 700 -- '$DEPLOY_PATH' && install -d -m 755 -- '$DEPLOY_PATH/web/static/pmtiles'"
if ! ssh "${SSH_OPTIONS[@]}" "$SSH_TARGET" \
    "cd '$DEPLOY_PATH' && test -f .env && chmod 600 .env"; then
    printf 'Place the application secrets in %s:%s/.env with mode 0600, then retry.\n' \
        "$SSH_TARGET" "$DEPLOY_PATH" >&2
    exit 1
fi

if [[ "$SKIP_BUILD" != "1" ]]; then
    docker buildx build \
        --platform "$BUILD_PLATFORM" \
        --tag "${HERMES_IMAGE}:${HERMES_TAG}" \
        --push \
        "$ROOT_DIR"
fi

TEMP_ENV="$(mktemp)"
trap 'rm -f "$TEMP_ENV"' EXIT
awk '!/^HERMES_TAG=/' "$ENV_FILE" > "$TEMP_ENV"
printf 'HERMES_TAG=%s\n' "$HERMES_TAG" >> "$TEMP_ENV"

scp "${SCP_OPTIONS[@]}" compose.yaml "$SSH_TARGET:$DEPLOY_PATH/"
scp "${SCP_OPTIONS[@]}" "$TEMP_ENV" "$SSH_TARGET:$DEPLOY_PATH/deploy.env"

ssh "${SSH_OPTIONS[@]}" "$SSH_TARGET" \
    "cd '$DEPLOY_PATH' && docker compose --env-file deploy.env pull && docker compose --env-file deploy.env up -d db"

printf 'Waiting up to %s seconds for the database to become healthy...\n' "$DEPLOY_HEALTH_TIMEOUT"
status=""
deadline=$((SECONDS + DEPLOY_HEALTH_TIMEOUT))
while (( SECONDS < deadline )); do
    status="$(ssh "${SSH_OPTIONS[@]}" "$SSH_TARGET" \
        "cd '$DEPLOY_PATH' && container_id=\$(docker compose --env-file deploy.env ps -q db) && test -n \"\$container_id\" && docker inspect --format '{{.State.Health.Status}}' \"\$container_id\"" \
        2>/dev/null || true)"
    if [[ "$status" == "healthy" ]]; then
        break
    fi
    sleep 3
done
if [[ "$status" != "healthy" ]]; then
    printf 'Database did not become healthy. Recent status and logs:\n' >&2
    ssh "${SSH_OPTIONS[@]}" "$SSH_TARGET" \
        "cd '$DEPLOY_PATH' && docker compose --env-file deploy.env ps && docker compose --env-file deploy.env logs --tail=100 db" >&2 || true
    exit 1
fi

for migration in "$ROOT_DIR"/migrations/*.sql; do
    printf 'Applying %s...\n' "$(basename "$migration")"
    ssh "${SSH_OPTIONS[@]}" "$SSH_TARGET" \
        "cd '$DEPLOY_PATH' && docker compose --env-file deploy.env exec -T db psql -U '$POSTGRES_USER' -d '$POSTGRES_DB' -v ON_ERROR_STOP=1 -q" \
        < "$migration"
done

ssh "${SSH_OPTIONS[@]}" "$SSH_TARGET" \
    "cd '$DEPLOY_PATH' && docker compose --env-file deploy.env up -d --remove-orphans"

printf 'Waiting up to %s seconds for the service to become healthy...\n' "$DEPLOY_HEALTH_TIMEOUT"
deadline=$((SECONDS + DEPLOY_HEALTH_TIMEOUT))
while (( SECONDS < deadline )); do
    status="$(ssh "${SSH_OPTIONS[@]}" "$SSH_TARGET" \
        "cd '$DEPLOY_PATH' && container_id=\$(docker compose --env-file deploy.env ps -q api) && test -n \"\$container_id\" && docker inspect --format '{{.State.Health.Status}}' \"\$container_id\"" \
        2>/dev/null || true)"
    if [[ "$status" == "healthy" ]]; then
        printf 'Deployed %s:%s to %s.\n' "$HERMES_IMAGE" "$HERMES_TAG" "$SSH_TARGET"
        exit 0
    fi
    sleep 3
done

printf 'Service did not become healthy. Recent status and logs:\n' >&2
ssh "${SSH_OPTIONS[@]}" "$SSH_TARGET" \
    "cd '$DEPLOY_PATH' && docker compose --env-file deploy.env ps && docker compose --env-file deploy.env logs --tail=100 api" >&2 || true
exit 1
