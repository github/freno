FROM golang:1.14-alpine

COPY . /go/src/github.com/github/freno
WORKDIR /go/src/github.com/github/freno

RUN go build -ldflags '-w -s' -o freno cmd/freno/main.go

FROM alpine

COPY --from=0 /go/src/github.com/github/freno/freno /usr/local/bin/freno
COPY conf/freno.conf.json /etc/freno.conf.json

RUN adduser --system freno
RUN mkdir -p /var/lib/freno && chown -R freno /var/lib/freno

USER freno
ENTRYPOINT ["freno"]
CMD ["--http"]
