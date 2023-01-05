# syntax=docker/dockerfile:1.3.1
FROM ubuntu:20.04 AS build
RUN apt-get -qq update && apt-get -qq install -y ca-certificates curl
EXPOSE 8081
EXPOSE 8080
WORKDIR /app
COPY --from=openfga/openfga:v0.2.5 /openfga /app/openfga
COPY --from=openfga/openfga:v0.2.5 /assets /app/assets
ENTRYPOINT ["/app/openfga"]