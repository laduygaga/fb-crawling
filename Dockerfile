FROM golang:1.21 as build

WORKDIR /golang
COPY . .

RUN CGO_ENABLED=0 go build -o fb-crawling-api main.go


FROM jrottenberg/ffmpeg:4.4-alpine

RUN ln -s /usr/local/bin/* /usr/bin/
RUN apk add --update-cache tzdata ca-certificates && rm -rf /var/cache/apk/*

COPY --from=build /golang/fb-crawling-api /usr/local/bin/fb-crawling-api

EXPOSE 5000
ENTRYPOINT ["/usr/local/bin/fb-crawling-api"]

