FROM golang:1.25-alpine AS build
WORKDIR /src
RUN apk add --no-cache git
COPY go.mod go.sum* ./
RUN go mod download || true
COPY . .
RUN CGO_ENABLED=0 go build -o /out/worker ./cmd/worker

FROM golang:1.25-alpine AS toolbuild
RUN apk add --no-cache git ca-certificates build-base libpcap-dev
ENV GOBIN=/tools CGO_ENABLED=1
RUN mkdir -p /tools
# Each tool is best-effort: failures here only mean the corresponding pipeline
# step will skip cleanly at runtime.
RUN go install github.com/projectdiscovery/subfinder/v2/cmd/subfinder@latest || echo "subfinder install failed"
RUN go install github.com/tomnomnom/assetfinder@latest || echo "assetfinder install failed"
RUN go install github.com/projectdiscovery/httpx/cmd/httpx@latest || echo "httpx install failed"
RUN go install github.com/projectdiscovery/naabu/v2/cmd/naabu@latest || echo "naabu install failed"
RUN go install github.com/projectdiscovery/katana/cmd/katana@latest || echo "katana install failed"
RUN go install github.com/lc/gau/v2/cmd/gau@latest || echo "gau install failed"
RUN go install github.com/ffuf/ffuf/v2@latest || echo "ffuf install failed"

FROM alpine:3.20
RUN apk add --no-cache \
    ca-certificates \
    chromium \
    nss \
    freetype \
    harfbuzz \
    ttf-freefont \
    libstdc++ \
    libpcap \
    && update-ca-certificates
ENV CHROMEDP_NO_SANDBOX=1 \
    CHROME_BIN=/usr/bin/chromium-browser \
    PATH=/usr/local/bin:$PATH
# Best-effort copy: missing tools are tolerated (pipeline step will fail/skip).
COPY --from=toolbuild /tools/ /usr/local/bin/
COPY --from=build /out/worker /usr/local/bin/worker
ENTRYPOINT ["/usr/local/bin/worker"]
