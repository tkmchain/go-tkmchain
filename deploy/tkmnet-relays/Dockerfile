FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates tor gosu curl && rm -rf /var/lib/apt/lists/* \
    && useradd --system --uid 10001 --home-dir /var/lib/tkmnet --shell /usr/sbin/nologin tkmrelay \
    && usermod -a -G debian-tor tkmrelay \
    && install -d -o tkmrelay -g tkmrelay -m 0700 /var/lib/tkmnet /etc/tkmnet \
    && install -d -o debian-tor -g debian-tor -m 0700 /var/lib/tor/data /var/lib/tor/hidden
COPY deploy/tkmnet-relays/bin/tkmnet-relay /usr/local/bin/tkmnet-relay
COPY deploy/tkmnet-relays/entrypoint.sh /usr/local/bin/entrypoint.sh
RUN chmod 0755 /usr/local/bin/entrypoint.sh
USER root
ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
