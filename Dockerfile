ARG DOCKER_REGISTRY
FROM ${DOCKER_REGISTRY}golang:1.18.1 AS golang
RUN go version

FROM ${DOCKER_REGISTRY}ubuntu:20.04 AS build-env

ARG http_proxy
ARG https_proxy
ARG no_proxy
ARG NO_PROXY

# Install general deps
RUN apt-get -qq update && apt-get -qq install -y ca-certificates curl git gcc build-essential 

# Set-up go
COPY --from=golang /usr/local/go/ /usr/local/go/
ENV PATH /usr/local/go/bin:$PATH
ENV GO111MODULE=on

RUN mkdir /src
WORKDIR /src

ARG GH_SSH_KEY
ARG GH_USERNAME
ARG GH_PASSWORD
ARG GOPROXY
ARG GOSUMDB
ARG GOPRIVATE=github.com/CanonicalLtd
COPY ./scripts/docker-github-auth.sh .
RUN ./docker-github-auth.sh

ARG GOMODMODE=readonly
# Cache modules needed in a docker layer to speed up subsequent builds
COPY go.mod .
COPY go.sum .
RUN [ "$GOMODMODE" = "vendor" ] || go mod download

COPY . .
# Set version
ARG GIT_COMMIT
ARG VERSION
RUN ./scripts/set-version.sh

ARG TAGS
RUN go build --tags "$TAGS" -mod $GOMODMODE -o jimmsrv ./cmd/jimmsrv

# Define a smaller single process image for deployment
FROM ${DOCKER_REGISTRY}ubuntu:20.04 AS deploy-env
RUN apt-get -qq update && apt-get -qq install -y ca-certificates postgresql-client
WORKDIR /root/
COPY --from=build-env /src/jimmsrv .
COPY --from=build-env /src/internal/dbmodel/sql ./sql/
CMD ["./jimmsrv", "./config.yaml"]

