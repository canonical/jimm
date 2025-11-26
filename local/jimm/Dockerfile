# Installing delve in the image to avoid running go install in running containers.
# Installing air to avoid breakages when we update our Go version before a new air image is available.
FROM golang:1.25 AS dev

RUN go install github.com/go-delve/delve/cmd/dlv@v1.25.2
RUN go install github.com/air-verse/air@v1.63.1

ENTRYPOINT [ "air" ]

# Create a target without air that we use in smoke tests.
FROM golang:1.25-bookworm AS smoke

COPY entrypoint.sh     /entry/entrypoint.sh

CMD [ "/entry/entrypoint.sh" ]
