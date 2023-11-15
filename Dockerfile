# syntax=docker/dockerfile:1.3.1
FROM ubuntu:20.04 as build-env
ARG GIT_COMMIT
ARG VERSION
ARG GO_VERSION
ARG ARCH
WORKDIR /usr/src/jimm
SHELL ["/bin/bash", "-c"]
COPY . .
RUN apt update && apt install wget gcc -y
RUN wget -L "https://golang.org/dl/go${GO_VERSION}.linux-${ARCH}.tar.gz"
RUN tar -C /usr/local -xzf "go${GO_VERSION}.linux-${ARCH}.tar.gz"
ENV PATH="${PATH}:/usr/local/go/bin"
RUN echo "${GIT_COMMIT}" | tee ./version/commit.txt
RUN echo "${VERSION}" | tee ./version/version.txt
RUN go build -tags version -o jimmsrv -v ./cmd/jimmsrv

# Define a smaller single process image for deployment
FROM ${DOCKER_REGISTRY}ubuntu:20.04 AS deploy-env
RUN apt-get -qq update && apt-get -qq install -y ca-certificates postgresql-client
WORKDIR /root/
COPY --from=build-env /usr/src/jimm/jimmsrv .
COPY --from=build-env /usr/src/jimm/internal/dbmodel/sql ./sql/
ENTRYPOINT [ "./jimmsrv" ]
CMD ["./config.yaml"]

