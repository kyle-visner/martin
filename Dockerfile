# syntax=docker/dockerfile:1
# Martin's MCP server as a distroless image (static binary, CA roots, no
# shell), the same shape as Magpie's. Typical use beside a Stellar Jay store:
#
#   docker run --network container:<store> \
#     -e JAYBASE_URL=http://127.0.0.1:8080 -e JAYBASE_TOKEN_FILE=/run/secrets/store_token \
#     -e MARTIN_MCP_TOKEN_FILE=/run/secrets/mcp_token \
#     martin --actor owner mcp --http :8879
#
# HTTP MCP refuses to start without MARTIN_MCP_TOKEN(_FILE).
FROM --platform=$BUILDPLATFORM golang:1.26.11-alpine AS build

ARG TARGETOS
ARG TARGETARCH
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build \
    -trimpath -ldflags="-s -w" -o /out/martin ./cmd/martin

FROM scratch
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build /out/martin /martin
USER 65532:65532
EXPOSE 8879
ENTRYPOINT ["/martin"]
CMD ["--actor", "owner", "mcp", "--http", ":8879"]
