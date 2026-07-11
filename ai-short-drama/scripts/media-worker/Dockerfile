# syntax=docker/dockerfile:1.7
ARG FFMPEG_VERSION=7.1.1
FROM mwader/static-ffmpeg:${FFMPEG_VERSION} AS ffmpeg

ARG NODE_VERSION=22.14.0-alpine3.21
FROM node:${NODE_VERSION}

ENV NODE_ENV=production \
    MEDIA_STORAGE_PATH=/data/storage \
    FFMPEG_PATH=/usr/local/bin/ffmpeg \
    FFPROBE_PATH=/usr/local/bin/ffprobe \
    MEDIA_WORKER_PORT=8090

WORKDIR /app
COPY scripts/media-worker/package.json ./package.json
RUN npm install --omit=dev --ignore-scripts --no-audit --no-fund \
    && npm cache clean --force

COPY --from=ffmpeg --chmod=0755 /ffmpeg /usr/local/bin/ffmpeg
COPY --from=ffmpeg --chmod=0755 /ffprobe /usr/local/bin/ffprobe
COPY scripts/media-worker/worker.js scripts/media-worker/ffmpeg-templates.js ./

RUN mkdir -p /data/storage \
    && chown -R node:node /app /data/storage

USER node
EXPOSE 8090
HEALTHCHECK --interval=15s --timeout=5s --start-period=10s --retries=4 \
  CMD node -e "fetch('http://127.0.0.1:8090/health').then(r=>{if(!r.ok)process.exit(1)}).catch(()=>process.exit(1))"

CMD ["node", "worker.js"]
