FROM golang:1.26-alpine AS build

WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/permission-center .

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=build /out/permission-center /app/permission-center
COPY --from=build /src/config/config.example.yaml /app/config/config.yaml
ENV CONFIG_FILE=/app/config/config.yaml
EXPOSE 8080 9090
ENTRYPOINT ["/app/permission-center"]
