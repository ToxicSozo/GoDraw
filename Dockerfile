FROM golang:1.22 AS builder
WORKDIR /app
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o reviewer-service ./cmd/server

FROM gcr.io/distroless/base-debian12
WORKDIR /app
COPY --from=builder /app/reviewer-service /app/reviewer-service
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/app/reviewer-service"]
