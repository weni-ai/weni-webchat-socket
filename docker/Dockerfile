# syntax = docker/dockerfile:1

ARG GO_VERSION="1.24"
ARG DEBIAN_VERSION="bookworm"

FROM golang:${GO_VERSION}-${DEBIAN_VERSION} AS base

ARG APP_UID=1000
ARG APP_GID=1000

ARG BUILD_DEPS="\
  gcc \
  git \
  curl"

ARG RUNTIME_DEPS="\
  gosu \
  curl \
  bash"

ARG WWC_PORT="8000"

ARG APP_VERSION="1.0"

# set environment variables
ENV PROJECT_PATH="/app"

ENV APPLICATION_NAME="weni-websocket"

ENV APP_VERSION=${APP_VERSION} \
  RUNTIME_DEPS=${RUNTIME_DEPS} \
  BUILD_DEPS=${BUILD_DEPS} \
  APP_UID=${APP_UID} \
  APP_GID=${APP_GID} \
  WWC_PORT=${WWC_PORT}

LABEL app=${APP_VERSION} \
  os="debian" \
  os.version="12" \
  name="${APPLICATION_NAME} ${APP_VERSION}" \
  description="${APPLICATION_NAME} image" \
  maintainer="${APPLICATION_NAME} Team"

RUN addgroup --gid "${APP_GID}" app_group \
  && useradd --system -m -d "${PROJECT_PATH}" -u "${APP_UID}" -g "${APP_GID}" app_user
  #&& adduser -S -h "${PROJECT_PATH}" -u "${APP_UID}" -G "app_group" app_user

# set work directory
WORKDIR ${PROJECT_PATH}

FROM base AS build

ARG SERVICE_TYPE="websocket"

RUN --mount=type=cache,target=/var/cache/apt,sharing=locked \
    --mount=type=cache,target=/var/lib/apt,sharing=locked \
  rm -f /etc/apt/apt.conf.d/docker-clean \
  && echo 'Binary::apt::APT::Keep-Downloaded-Packages "true";' > /etc/apt/apt.conf.d/keep-cache \
  && if [ ! "x${BUILD_DEPS}" = "x" ] ; then apt-get update \
  && apt-get install -y --no-install-recommends ${BUILD_DEPS} ; fi

# Copy and download dependency using go mod
COPY go.mod go.sum .

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
  go mod download -x

# Copy the code into the container
COPY --chown=app_user:app_group . .

# Build the application based on SERVICE_TYPE
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
  go build -o ${APPLICATION_NAME} ./api

FROM base

#RUN if [ ! "x${RUNTIME_DEPS}" = "x" ] ; then apt-get update ; apt-get -y install ${RUNTIME_DEPS}; fi
RUN --mount=type=cache,target=/var/cache/apt,sharing=locked \
    --mount=type=cache,target=/var/lib/apt,sharing=locked \
  rm -f /etc/apt/apt.conf.d/docker-clean \
  && echo 'Binary::apt::APT::Keep-Downloaded-Packages "true";' > /etc/apt/apt.conf.d/keep-cache \
  && apt-get update \
  && SUDO_FORCE_REMOVE=yes apt-get remove --purge -y ${BUILD_DEPS} \
  && apt-get autoremove -y \
  && apt-get install -y --no-install-recommends ${RUNTIME_DEPS} \
  && rm -rf /usr/share/man /usr/share/doc

# copy project
#COPY --from=build --chown=app_user:app_group ${PROJECT_PATH}/${APPLICATION_NAME}* /bin
COPY --from=build --chown=root:root ${PROJECT_PATH}/${APPLICATION_NAME}* /bin

COPY docker/docker-entrypoint.sh /

USER ${APP_UID}:${APP_GID}

ENTRYPOINT ["/docker-entrypoint.sh"]

CMD ["start"]

HEALTHCHECK --interval=15s --timeout=20s --start-period=60s \
  CMD /docker-entrypoint.sh healthcheck
