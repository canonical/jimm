# Installing delve in the image to avoid running go install in running containers
FROM cosmtrek/air:latest AS dev

RUN go install github.com/go-delve/delve/cmd/dlv@latest

ENTRYPOINT [ "air" ]

# Create a target without air that we use in smoke tests.
FROM golang:1.24-bookworm as smoke

COPY entrypoint.sh     /entry/entrypoint.sh

CMD [ "/entry/entrypoint.sh" ]
