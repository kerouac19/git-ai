FROM golang:1.26-alpine AS build
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o git-ai-server ./cmd/server

FROM gcr.io/distroless/static-debian12
WORKDIR /app
COPY --from=build /app/git-ai-server .
EXPOSE 3000
ENTRYPOINT ["/app/git-ai-server"]
