FROM golang:1.22-alpine AS build
WORKDIR /src
COPY . .
RUN go mod tidy && CGO_ENABLED=0 go build -o /out/server .

FROM alpine:3.20
RUN apk add --no-cache ffmpeg ca-certificates
COPY --from=build /out/server /usr/local/bin/server
ENV PORT=8080
EXPOSE 8080
USER nobody
CMD ["server"]
