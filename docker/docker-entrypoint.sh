#!/bin/bash

set -e

bootstrap_conf(){
	find "${PROJECT_PATH}" -not -user "${APP_UID}" -exec chown "${APP_UID}:${APP_GID}" {} \+
}

bootstrap_conf

if [[ "start" == "$1" ]]; then
	exec su-exec "${APP_UID}:${APP_GID}" "./${APPLICATION_NAME}"
elif [[ "healthcheck" == "$1" ]]; then
	su-exec "${APP_UID}:${APP_GID}" curl -SsLf "http://127.0.0.1:${WWC_PORT}/healthcheck" -o /tmp/null --connect-timeout 3 --max-time 20 -w "%{http_code} %{http_version} %{response_code} %{time_total}\n" || exit 1
	exit 0
fi

exec "$@"
