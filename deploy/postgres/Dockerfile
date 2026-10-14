# Postgres 18 dengan WAL-G untuk full backup harian + WAL kontinu ke object storage di luar VPS
# (NFR-07, Technical Guideline 14.7). Tag image ini terpisah dari tag aplikasi agar deploy aplikasi
# tidak me-restart database.
FROM postgres:18.0-bookworm

ARG WALG_VERSION=v3.0.8
ARG WALG_SHA256=f30544c5ce93cf83b87578e3c4a2e9c0e0ffc3d160ef89ecddaf75f397d98deb

RUN apt-get update \
 && apt-get install -y --no-install-recommends ca-certificates curl \
 && curl -fsSL -o /usr/local/bin/wal-g "https://github.com/wal-g/wal-g/releases/download/${WALG_VERSION}/wal-g-pg-22.04-amd64" \
 && echo "${WALG_SHA256}  /usr/local/bin/wal-g" | sha256sum -c - \
 && chmod 0755 /usr/local/bin/wal-g \
 && apt-get purge -y --auto-remove \
 && rm -rf /var/lib/apt/lists/*

COPY backup-loop.sh /usr/local/bin/backup-loop
COPY backup-now.sh /usr/local/bin/backup-now
RUN chmod 0755 /usr/local/bin/backup-loop /usr/local/bin/backup-now
