FROM golang:1.24-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY *.go ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /bot .

FROM gcr.io/distroless/static
COPY --from=build /bot /bot
ENTRYPOINT ["/bot"]
