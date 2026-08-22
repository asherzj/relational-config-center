FROM golang:1.27-alpine AS build

WORKDIR /src/admin
COPY admin/go.mod admin/go.sum ./
RUN go mod download
COPY admin/ ./
RUN CGO_ENABLED=0 go build -trimpath -o /out/admin ./cmd/admin

FROM alpine:3.22
RUN addgroup -S rcc && adduser -S -G rcc rcc
USER rcc
COPY --from=build /out/admin /usr/local/bin/admin
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/admin"]
