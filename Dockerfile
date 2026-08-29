ARG GO_VER=1.26
ARG ALPINE_VER=3.22

FROM golang:${GO_VER}-alpine${ALPINE_VER} AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY cmd/ cmd/
COPY internal/ internal/
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/karmabot ./cmd/karmabot

FROM alpine:${ALPINE_VER}

RUN adduser -D -u 10001 karmabot
COPY --from=builder /out/karmabot /usr/local/bin/karmabot

RUN mkdir -p /data

# 3. Change ownership of the directory to the non-root user
RUN chown -R karmabot:karmabot /data

USER karmabot
VOLUME /data
ENV KARMABOT_DB_PATH=/data/karmabot.db

ENTRYPOINT ["karmabot"]
