FROM golang:1.26.1 as builder
COPY go.mod go.sum /go/src/github.com/oybek/ho/
WORKDIR /go/src/github.com/oybek/ho
RUN go mod download
COPY . /go/src/github.com/oybek/ho
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o build/ho github.com/oybek/ho

FROM alpine/curl
RUN apk add --no-cache ca-certificates && update-ca-certificates
COPY --from=builder /go/src/github.com/oybek/ho/build/ho /usr/bin/ho
EXPOSE 8080 8080
ENTRYPOINT ["/usr/bin/ho"]
