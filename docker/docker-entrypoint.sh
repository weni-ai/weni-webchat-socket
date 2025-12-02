#!/bin/bash

set -e

export PREFIX_ENV="${PREFIX_ENV:-"WWS"}"

################################################################################
# Get variable using a prefix
# Globals:
# 	PREFIX_ENV: Prefix used on variables
# Arguments:
# 	$1: Variable name to get the content, in format "${PREFIX_ENV}_$1"
# 	$2: Default value, if variable is empty or not exist
# Outputs:
# 	Output is the content of prefixed variable
# Returns:
# 	Nothing.
################################################################################
function get_env() {
	local env_name="${PREFIX_ENV}_${1}"
	if [ "${!env_name}" != "" ]; then
		echo -n "${!env_name}"
	else
		echo -n "${2}"
	fi
}

################################################################################
# Set variable using a prefix.  The variable is set in global context.
# Globals:
# 	PREFIX_ENV: Prefix used on variables
# Arguments:
# 	$1: Variable name to set the content, in format "${PREFIX_ENV}_$1"
# Outputs:
# 	Nothing
# Returns:
# 	Nothing.
################################################################################
function set_env() {
	export "${PREFIX_ENV}_${1}"
}

export GOSU_ID="$(get_env UID app_user):$(get_env GID app_group)"

################################################################################
# Execute gosu to change user and group uid, but works with exec and more
# friendly to nonroot. This is used to exec the same command when root or a
# normal execute a command on a container.
# If the inicial argument after the ID is exec, this function will try to be
# compatible with exec of bash.
# Globals:
# 	${PREFIX_ENV}_GOSU_ALLOW_ID: Default 0. If id 0 not has some kind of cap drop, set to something not equal to 0 and not empty.
# Arguments:
# 	$@: Same argument as gosu
# Outputs:
# 	Output the same stdout and stderr of executed program of command line arguments
# Returns:
# 	Return the same return code of executed program of command line arguments
################################################################################
do_gosu() {
	user="$1"
	shift 1

	is_exec="false"
	if [ "$1" = "exec" ]; then
		is_exec="true"
		shift 1
	fi

	# If user is 0, he can change uid and gid
	if [ "$(id -u)" = "$(get_env GOSU_ALLOW_ID '0')" ]; then
		if [ "${is_exec}" = "true" ]; then
			exec gosu "${user}" "$@"
		else
			gosu "${user}" "$@"
			return "$?"
		fi
	else
		if [ "${is_exec}" = "true" ]; then
			exec "$@"
		else
			eval '"$@"'
			return "$?"
		fi
	fi
}

bootstrap_conf() {
	if [ "$(get_env TZ)" ]; then
		# shellcheck disable=SC2005
		ln -snf "/usr/share/zoneinfo/$(get_env TZ)" /etc/localtime &&
			echo "$(get_env TZ)" > /etc/timezone
	fi
	if [ "$(get_env DO_CHOWN)" = "true" ]; then
		find "$(get_env WORK_DIR '/app')" \
			-not -user "$(get_env UID app_user)" \
			-exec chown "${GOSU_ID}" {} \+ || true
	fi
}

bootstrap_conf

if [[ "start" == "$1" ]]; then
	do_gosu "${GOSU_ID}" exec "${APPLICATION_NAME}"
elif [[ "healthcheck" == "$1" ]]; then
	do_gosu "${GOSU_ID}" curl -SsLf "http://127.0.0.1:${WWC_PORT}/healthcheck" -o /tmp/null --connect-timeout 3 --max-time 20 -w "%{http_code} %{http_version} %{response_code} %{time_total}\n" || exit 1
	exit 0
fi

exec "$@"
