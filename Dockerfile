FROM golang:1.27-trixie AS build

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ENV CGO_ENABLED=0
ENV GOCACHE=/go/cache

RUN --mount=type=cache,target=/go/cache/ go build -trimpath -tags timetzdata -o splitgo ./cmd/splitgo

RUN mkdir /database


FROM scratch

COPY --from=build /build/splitgo /bin/splitgo
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build --chown=65534:65534 /database/ /database/

ENV DATABASE_DIR=/database
ENV LISTEN_ADDR=:8080
EXPOSE 8080

USER 65534:65534

ENTRYPOINT ["/bin/splitgo"]
