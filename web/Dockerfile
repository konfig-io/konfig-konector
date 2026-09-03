# konfig-konector landing page — static site embedded in a single Go binary.
FROM golang:1.22-alpine AS build
WORKDIR /src
COPY go.mod ./
COPY main.go ./
COPY static/ static/
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /web .

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /web /web
ENV PORT=8080
EXPOSE 8080
USER nonroot
ENTRYPOINT ["/web"]
