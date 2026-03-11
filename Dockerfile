FROM golang:1.24-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o zone-latency-client .

FROM gcr.io/distroless/static:nonroot
COPY --from=builder /app/zone-latency-client /zone-latency-client
USER nonroot:nonroot
ENTRYPOINT ["/zone-latency-client"]
