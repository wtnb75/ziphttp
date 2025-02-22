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

from release

(TBD)

from source

```sh
# go install github.com/wtnb75/ziphttp@main
```

from docker

```sh
# docker pull ghcr.io/wtnb75/ziphttp:main
```

## Run

- serve a zipfile port 8888
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
