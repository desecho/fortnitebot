FROM --platform=$BUILDPLATFORM golang:1.23-alpine AS build

WORKDIR /src

COPY go.mod ./
COPY main.go ./

ARG TARGETOS=linux
ARG TARGETARCH

RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
	go build -trimpath -ldflags="-s -w" -o /out/fortnitebot .

FROM alpine:3.21

WORKDIR /app

RUN apk add --no-cache ca-certificates \
	&& adduser -D -u 10001 bot

COPY --from=build /out/fortnitebot /app/fortnitebot
COPY --chown=bot:bot players.json /app/players.json

RUN chown bot:bot /app/fortnitebot \
	&& chmod 755 /app/fortnitebot

USER bot

ENV PLAYERS_FILE=/app/players.json

ENTRYPOINT ["/app/fortnitebot"]
