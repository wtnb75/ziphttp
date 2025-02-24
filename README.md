# ziphttp: A simple HTTP server for serving files from a ZIP archive

ziphttp is a lightweight and easy-to-use HTTP server that allows you to serve static files from a ZIP archive over the internet. It's perfect for sharing files without setting up a full web server.

## Features

- **Serves files from a ZIP archive**: ziphttp extracts files from a ZIP archive and serves them via HTTP.
- **Easy to use**: Simply provide the path to your ZIP file, and ziphttp will start serving it on a specified port.
- **Supports multiple files and directories**: You can access individual files within the ZIP archive.
- **Cross-platform**: Works on Windows, macOS, and Linux.
- **Serves compressed files with lowest CPU load**: ziphttp send contents without decompress stream if it is acceptable.
- **Small footprint**: size of container image is only <10MB. serving files are also compressed as you can see.
- **Make single executable**: ziphttp can create self-extract zip with ziphttp itself. generated binary runs webserver using its own contents.
- **Client-side Cache friendly**: ziphttp serves static files, send response with `ETag` header based on checksum value in zip file. ziphttp supports conditional GET with `If-None-Match` header.

## Installation

from release: download binary from [releases](https://github.com/wtnb75/ziphttp/releases)

```sh
# export GH_TOKEN=.... <- set your token or run "gh auth login"
# version=$(gh release list -R wtnb75/ziphttp -L 1 --json name -q '.[].name')
# gh release download -R wtnb75/ziphttp ${version} -p "*$(uname -s)_$(uname -m)"
# chmod +x ziphttp_$(uname -s)_$(uname -m)
```

from source

```sh
# go install github.com/wtnb75/ziphttp@main
```

from docker

```sh
# docker pull ghcr.io/wtnb75/ziphttp:main
```

## Run

- serve a zipfile on port 8888
    - `ziphttp webserver -f your-zip.zip -l :8888`
- optimize zip with zopfli compression
    - `ziphttp zopflizip -f new-zip.zip [directory or file or .zip]...`
- make single executable binary contains zip and the server
    - `ziphttp zopflizip -f newserver --self [directory or file or .zip]...`
- boot the binary
    - `./newserver webserver --self -l :8888`
- load zip in-memory. no storage access required after initialize was finished
    - `ziphttp webserver -f your-zip.zip -l :8888 --in-memory`
    - `./newserver webserver --self --in-memory -l :8888`

# CookBook

## hugo

- build static site to public/
    - `hugo --minify`
- archive static site to single .zip
    - `ziphttp zopflizip -f hugo.zip -s public/`
- serve static site -> http://localhost:3000/
    - `ziphttp webserver -f hugo.zip`
- or self-extract .zip and run -> http://localhost:3000/
    - `ziphttp zopflizip -f hugo.run -s public/ --self`
    - `./hugo.run webserver --self`

as docker:

```Dockerfile
FROM alpine:3 AS build
RUN apk add hugo
ADD https://github.com/wtnb75/ziphttp/releases/download/v0.0.1/ziphttp_Linux_x86_64 /ziphttp
RUN chmod 755 /ziphttp
COPY . /app
RUN cd /app && hugo --minify && /ziphttp zopflizip --self -f /hugo.run -s public/

FROM scratch
COPY --from=build /hugo.run /
EXPOSE 3000
ENTRYPOINT ["/hugo.run"]
CMD ["webserver", "--self"]
```

## docker compose + traefik

```yaml
services:
  traefik:
    image: traefik:v2
    volumes:
    - /var/run/docker.sock:/var/run/docker.sock:ro
    command:
    - --providers.docker=true
    - --providers.docker.exposedbydefault=false
    - --entrypoint.web.address=:80
    network_mode: host
  static:   # http://localhost/static/
    image: ghcr.io/wtnb75/ziphttp:main
    volumes:
    - ./path/to/archive.zip:/archive.zip:ro
    environment:
      ZIPHTTP_ARCHIVE: /archive.zip
    command:
    - /ziphttp
    - webserver
    - --addprefix
    - /static
    labels:
      traefik.enable: "true"
      traefik.http.services.static.loadbalancer.server.port: "3000"
      traefik.http.routers.static.rule: "PathPrefix(`/static`)"
      traefik.http.routers.static.entrypoints: web
```
