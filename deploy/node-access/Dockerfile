FROM debian:bookworm-slim AS build

ARG KAMAILIO_VERSION=6.1.3

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        ca-certificates build-essential bison flex git pkg-config \
        libcurl4-openssl-dev libhiredis-dev libjansson-dev libpcre2-dev \
        libssl-dev libxml2-dev \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /usr/src
RUN git clone --branch "${KAMAILIO_VERSION}" --depth 1 \
      https://github.com/kamailio/kamailio.git kamailio \
    && test "$(git -C kamailio describe --tags --exact-match)" = "${KAMAILIO_VERSION}"

COPY deploy/kamailio/modules/gb28181 /usr/src/kamailio/src/modules/gb28181
WORKDIR /usr/src/kamailio
RUN make cfg prefix=/opt/kamailio \
      include_modules="auth pv registrar sl tm usrloc xhttp gb28181" \
    && make -j"$(nproc)" all \
    && make install

FROM debian:bookworm-slim

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        ca-certificates curl libcurl4 libhiredis0.14 libjansson4 libpcre2-8-0 \
        libssl3 libxml2 \
    && rm -rf /var/lib/apt/lists/* \
    && groupadd --system --gid 10001 kamailio \
    && useradd --system --uid 10001 --gid 10001 --no-create-home kamailio \
    && mkdir -p /var/run/kamailio \
    && chown kamailio:kamailio /var/run/kamailio

COPY --from=build /opt/kamailio /opt/kamailio
COPY deploy/kamailio/kamailio.cfg /etc/kamailio/kamailio.cfg

USER 10001:10001
EXPOSE 5060/udp 8090/tcp
HEALTHCHECK --interval=10s --timeout=3s --start-period=10s --retries=3 \
  CMD curl --fail --silent http://127.0.0.1:8090/readyz >/dev/null || exit 1
ENTRYPOINT ["/opt/kamailio/sbin/kamailio", "-DD", "-E", "-f", "/etc/kamailio/kamailio.cfg"]
