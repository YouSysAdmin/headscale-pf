ARG HEADSCALE_VERSION="0.26.1"
# latest
FROM ghcr.io/juanfont/headscale:${HEADSCALE_VERSION} AS headscale-src

FROM alpine:3.20
RUN apk add --no-cache ca-certificates

COPY --from=headscale-src /ko-app/headscale /usr/bin/headscale
COPY headscale-pf /usr/bin/headscale-pf

COPY build/docker/entrypoint.sh /usr/local/bin/entrypoint.sh
RUN chmod +x /usr/bin/headscale /usr/bin/headscale-pf /usr/local/bin/entrypoint.sh

ENV OUTPUT_POLICY=/work/policy.json \
    SOURCE=jc

WORKDIR /work
ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
