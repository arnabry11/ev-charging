#!/bin/bash
set -euo pipefail

if [ "${SKIP_DB_PREPARE:-0}" != "1" ]; then
  bundle exec rails db:prepare
fi

exec "$@"
