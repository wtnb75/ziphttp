FROM scratch
ARG EXT=
COPY ziphttp${EXT} /ziphttp
ENTRYPOINT ["/ziphttp"]
