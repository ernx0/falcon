FROM alpine:3.20
RUN apk add --no-cache curl postgresql16-client
CMD sh -c 'echo "=== ENV ==="; env | sort; echo "=== DONE ==="; sleep 600'
