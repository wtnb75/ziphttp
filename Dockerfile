FROM scratch
COPY ziphttp /
ENTRYPOINT ["/ziphttp"]
