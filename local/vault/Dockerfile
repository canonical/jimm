FROM hashicorp/vault:latest

# Add jq to make scripting the calls a bit easier
# ref: https://stedolan.github.io/jq/
RUN apk add --no-cache bash jq

# Add our policy and entrypoint
COPY policy.hcl                   /vault/policy.hcl
COPY entrypoint.sh                /vault/entrypoint.sh

EXPOSE 8200

ENTRYPOINT [ "/vault/entrypoint.sh" ]

HEALTHCHECK \
    --start-period=5s \
    --interval=1s \
    --timeout=1s \
    --retries=30 \
        CMD [ "/bin/sh", "-c", "[ -f /tmp/healthy ]" ]
