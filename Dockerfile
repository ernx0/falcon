FROM alpine:3.20
RUN apk add --no-cache curl postgresql16-client
COPY probe.sh /probe.sh
RUN chmod +x /probe.sh
CMD ["/probe.sh"]
