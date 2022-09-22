# syntax=docker/dockerfile:1.3.1
FROM ubuntu:20.04 AS build
SHELL ["/bin/bash", "-c"]
ENV GVM_VERSION=master
COPY ./go.mod ./go.mod
RUN apt-get update && \
    apt-get -y install gcc bison binutils make git gcc curl build-essential mercurial ca-certificates
RUN bash < <(curl -SL -v https://raw.githubusercontent.com/moovweb/gvm/${GVM_VERSION}/binscripts/gvm-installer) && \
    source /root/.gvm/scripts/gvm && \
    gvm install go$(cat go.mod | sed -n "/^go/p" | cut -d ' ' -f 2)  -B && \
    gvm use go$(cat go.mod | sed -n "/^go/p" | cut -d ' ' -f 2)  --default


FROM build as build-with-github-auth
ARG AUTH_TYPE
ARG GIT_COMMIT
ARG VERSION
WORKDIR /usr/src/jimm
SHELL ["/bin/bash", "-c"]
COPY . .
RUN --mount=type=secret,id=ghuser \
    --mount=type=secret,id=ghpat \ 
    --mount=type=ssh \ 
    if [ "$AUTH_TYPE" = "pat" ]; then \
    echo "machine github.com login $(cat /run/secrets/ghuser) password $(cat /run/secrets/ghpat)" > $HOME/.netrc && \
    echo "PAT auth selected"; \
    elif [ "$AUTH_TYPE" = "ssh" ]; then \
    git config --global user.name $(cat /run/secrets/ghuser) && \
    mkdir -p -m 0600 ~/.ssh && \
    echo $(ssh-keyscan github.com) >> ~/.ssh/known_hosts && \
    git config --global --add url."git@github.com:".insteadOf "https://github.com/" && \
    echo "SSH auth selected"; \
    fi
RUN echo "${GIT_COMMIT}" | tee ./version/commit.txt
RUN echo "${VERSION}" | tee ./version/version.txt
RUN --mount=type=ssh source /root/.gvm/scripts/gvm && go mod vendor
RUN --mount=type=ssh source /root/.gvm/scripts/gvm && go build -o jimmsrv -race -v -a -mod vendor ./cmd/jimmsrv

# Define a smaller single process image for deployment
FROM ${DOCKER_REGISTRY}ubuntu:20.04 AS deploy-env
RUN apt-get -qq update && apt-get -qq install -y ca-certificates postgresql-client
WORKDIR /root/
COPY --from=build-with-github-auth /usr/src/jimm/jimmsrv .
COPY --from=build-with-github-auth /usr/src/jimm/internal/dbmodel/sql ./sql/
ENTRYPOINT [ "./jimmsrv" ]
CMD ["./config.yaml"]

