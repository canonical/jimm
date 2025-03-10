# Installing delve in the image to avoid running go install in running containers.
# Installing air to avoid breakages when we update our Go version before a new air image is available.
FROM golang:1.24 AS dev

RUN go install github.com/go-delve/delve/cmd/dlv@latest
RUN go install github.com/air-verse/air@latest

ENTRYPOINT [ "air" ]

# Create a target without air that we use in smoke tests.
FROM golang:1.24-bookworm as smoke

COPY entrypoint.sh     /entry/entrypoint.sh

CMD [ "/entry/entrypoint.sh" ]
