# syntax=docker/dockerfile:1.3.1
FROM ubuntu:20.04 AS build

# Install some tools necessary for health checks and setup.
RUN apt-get -qq update && apt-get -qq install -y ca-certificates curl wget postgresql-client

EXPOSE 8081
EXPOSE 8080

WORKDIR /app

# Copy OpenFGA binaries from upstream image
COPY --from=openfga/openfga:v1.2.0 /openfga /app/openfga
COPY --from=openfga/openfga:v1.2.0 /assets /app/assets

COPY entrypoint.sh                /app/entrypoint.sh

ENTRYPOINT [ "/app/entrypoint.sh" ]

HEALTHCHECK \
    --start-period=5s \
    --interval=1s \
    --timeout=5s \
    --retries=10 \
        CMD [ "curl", "http://0.0.0.0:8080/healthz" ]
