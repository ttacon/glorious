FROM golang AS build
ENV GO111MODULE=on

WORKDIR /go/build/
COPY . .

RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -o glo

# Now pull in what we need.
FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=build /go/build/glo .
ENTRYPOINT [ "./glo" ]
CMD []
