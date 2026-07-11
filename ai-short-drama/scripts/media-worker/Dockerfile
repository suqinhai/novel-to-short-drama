ARG N8N_VERSION=2.4.4
FROM docker.n8n.io/n8nio/n8n:${N8N_VERSION}

ARG FFMPEG_VERSION=6.1.2-r2
ARG APK_TOOLS_STATIC_VERSION=2.14.10-r0
ARG APK_TOOLS_STATIC_SHA256=c86e3822764e5fe19f41ce2e13553e48cac1ea4e74f858338e8d44bf0b616b61

USER root
RUN set -eux; \
    apk_url="https://dl-cdn.alpinelinux.org/alpine/v3.22/main/x86_64/apk-tools-static-${APK_TOOLS_STATIC_VERSION}.apk"; \
    wget -q -T 60 -O /tmp/apk-tools-static.apk "$apk_url"; \
    echo "${APK_TOOLS_STATIC_SHA256}  /tmp/apk-tools-static.apk" | sha256sum -c -; \
    mkdir -p /tmp/apk-static; \
    tar -xzf /tmp/apk-tools-static.apk -C /tmp/apk-static; \
    /tmp/apk-static/sbin/apk.static add --no-cache "ffmpeg=${FFMPEG_VERSION}"; \
    ffmpeg -version; \
    ffprobe -version; \
    rm -rf /tmp/apk-static /tmp/apk-tools-static.apk

ENV NODE_ENV=production \
    MEDIA_STORAGE_PATH=/data/storage \
    FFMPEG_PATH=/usr/bin/ffmpeg \
    FFPROBE_PATH=/usr/bin/ffprobe \
    MEDIA_WORKER_PORT=8090

WORKDIR /app
COPY scripts/media-worker/package.json ./package.json
RUN npm install --omit=dev --ignore-scripts --no-audit --no-fund \
    && npm cache clean --force

COPY scripts/media-worker/worker.js scripts/media-worker/ffmpeg-templates.js ./

RUN mkdir -p /data/storage \
    && chown -R node:node /app /data/storage

USER node
EXPOSE 8090
HEALTHCHECK --interval=15s --timeout=5s --start-period=10s --retries=4 \
  CMD node -e "fetch('http://127.0.0.1:8090/health').then(r=>{if(!r.ok)process.exit(1)}).catch(()=>process.exit(1))"

ENTRYPOINT ["/sbin/tini", "--"]
CMD ["node", "worker.js"]
