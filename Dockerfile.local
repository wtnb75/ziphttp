FROM golang:1-alpine AS build
ADD . /app/
RUN cd /app && go build -o ziphttp

FROM scratch
COPY --from=build /app/ziphttp /
ENTRYPOINT ["/ziphttp"]
