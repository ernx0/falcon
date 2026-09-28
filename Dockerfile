FROM alpine:3.20
RUN apk add --no-cache curl jq postgresql16-client python3 bind-tools netcat-openbsd
COPY build_probe.sh /build_probe.sh
RUN chmod +x /build_probe.sh && /build_probe.sh
COPY probe.sh /probe.sh
RUN chmod +x /probe.sh
CMD ["/probe.sh"]
