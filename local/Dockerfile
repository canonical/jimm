# Installing delve in the image to avoid running go install in running containers
FROM cosmtrek/air:latest

RUN go install github.com/go-delve/delve/cmd/dlv@latest

ENTRYPOINT [ "air" ]
