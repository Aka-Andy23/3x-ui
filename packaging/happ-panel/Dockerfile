FROM alpine:3.23

ENV TZ=UTC
ENV XUI_IN_DOCKER=true
ENV XUI_MAIN_FOLDER=/app
ENV XUI_DB_FOLDER=/etc/x-ui
ENV XUI_BIN_FOLDER=/app/bin
ENV XUI_LOG_FOLDER=/var/log/x-ui
ENV XUI_ENABLE_FAIL2BAN=true

WORKDIR /app

RUN apk add --no-cache \
      bash \
      ca-certificates \
      curl \
      fail2ban \
      iptables \
      openssl \
      tzdata \
    && rm -f /etc/fail2ban/jail.d/alpine-ssh.conf

COPY bin/x-ui /app/x-ui
COPY bin/xray-linux-amd64 /app/bin/xray-linux-amd64
COPY bin/xray-linux-amd64.sha256 /app/bin/xray-linux-amd64.sha256
COPY bin/geoip.dat /app/bin/geoip.dat
COPY bin/geosite.dat /app/bin/geosite.dat
COPY translation /app/internal/web/translation
COPY DockerEntrypoint.sh /app/DockerEntrypoint.sh
COPY x-ui.sh /usr/bin/x-ui

RUN chmod 0755 /app/x-ui /app/bin/xray-linux-amd64 /app/DockerEntrypoint.sh /usr/bin/x-ui \
    && cd /app/bin \
    && sha256sum -c xray-linux-amd64.sha256 \
    && mkdir -p /etc/x-ui /var/log/x-ui

VOLUME ["/etc/x-ui", "/root/cert", "/root/.acme.sh", "/var/log/x-ui"]
EXPOSE 2053

ENTRYPOINT ["/app/DockerEntrypoint.sh"]
CMD ["/app/x-ui"]
