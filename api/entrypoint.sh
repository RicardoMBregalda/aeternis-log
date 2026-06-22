#!/bin/sh
set -e

# Schema changes are a deploy step, not a boot side effect. Environments that opt
# in (RUN_MIGRATIONS=true, e.g. docker-compose) apply pending migrations here,
# before the API starts. In Kubernetes this runs instead as a separate
# pre-upgrade Job, with RUN_MIGRATIONS left false so the API only asserts the
# schema version at boot and never mutates it.
if [ "${RUN_MIGRATIONS:-false}" = "true" ]; then
  echo "entrypoint: applying database migrations (RUN_MIGRATIONS=true)"
  /app/aeternislog-migrate
fi

exec /app/aeternislog-api "$@"
