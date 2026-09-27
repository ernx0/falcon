FROM alpine:3.20
RUN apk add --no-cache curl jq socat postgresql16-client python3 bind-tools
COPY probe.sh /probe.sh
RUN chmod +x /probe.sh
CMD ["/probe.sh"]
