# syntax=docker/dockerfile:1.3.1

# The published hook-service image is a distroless rock with no shell,
# so we build a plain Ubuntu image and copy the binary out of it.
FROM ghcr.io/canonical/hook-service:latest AS hook-service

FROM ubuntu:24.04

# Install the postgres client used for migrations and seeding.
RUN apt-get -qq update && apt-get -qq install -y ca-certificates postgresql-client

COPY --from=hook-service /usr/bin/hook-service /usr/bin/hook-service

WORKDIR /app

COPY entrypoint.sh /app/entrypoint.sh
RUN chmod +x /app/entrypoint.sh

ENTRYPOINT [ "/app/entrypoint.sh" ]

# Opt to check for a file rather than the hook-service itself
# because we perform migrations and seeding after starting the server.
HEALTHCHECK \
    --start-period=5s \
    --interval=1s \
    --timeout=1s \
    --retries=30 \
    CMD [ "/bin/sh", "-c", "[ -f /tmp/healthy ]" ]
