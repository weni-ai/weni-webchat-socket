FROM golang:1.17.11-alpine3.16 AS base

ARG APP_UID=1000
ARG APP_GID=1000

ARG BUILD_DEPS="\
  gcc \
  git \
  curl \
  tar"

ARG RUNTIME_DEPS="\
  curl \
  su-exec \
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

LABEL app=${VERSION} \
  os="alpine" \
  os.version="3.13" \
  name="${APPLICATION_NAME} ${APP_VERSION}" \
  description="${APPLICATION_NAME} image" \
  maintainer="${APPLICATION_NAME} Team"

RUN addgroup --gid "${APP_GID}" app_group \
  && adduser -S -h ${PROJECT_PATH} -u "${APP_UID}" -G "app_group" app_user

# set work directory
WORKDIR ${PROJECT_PATH}

FROM base AS build

RUN if [ ! "x${BUILD_DEPS}" = "x" ] ; then apk add --no-cache ${BUILD_DEPS}; fi

# Copy and download dependency using go mod
COPY go.mod .
COPY go.sum .
RUN go mod download

# Copy the code into the container
COPY --chown=app_user:app_group . . 

# Build the application
RUN go build -o ${APPLICATION_NAME} ./api

FROM base

# copy project
COPY --from=build --chown=app_user:app_group ${PROJECT_PATH}/${APPLICATION_NAME} ${PROJECT_PATH}

RUN if [ ! "x${RUNTIME_DEPS}" = "x" ] ; then apk add --no-cache ${RUNTIME_DEPS}; fi

COPY docker/docker-entrypoint.sh .

ENTRYPOINT ["/app/docker-entrypoint.sh"]

CMD ["start"]

HEALTHCHECK --interval=15s --timeout=20s --start-period=60s \
  CMD "${PROJECT_PATH}"/docker-entrypoint.sh healthcheck
