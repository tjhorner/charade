FROM golang:1.16 AS builder

WORKDIR /build
COPY . .

RUN go get -d -v ./...
RUN go build -o /bot -ldflags "-linkmode external -extldflags -static" -a *.go

# We use the distroless/static image since it includes a list of CAs and tzinfo, but is also very slim
FROM gcr.io/distroless/static:8bef63d2c8654ff89358430c7df5778162ab6027
COPY --from=builder /bot /bot
CMD ["/bot"]