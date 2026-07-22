FROM node:22-alpine

ENV NODE_ENV=production \
    VEO_ADAPTER_PORT=8091 \
    VEO_LOCATION=us-central1 \
    VEO_TASK_DIR=/data/tasks

WORKDIR /app
COPY scripts/veo-adapter/package.json ./package.json
COPY scripts/veo-adapter/lib.js scripts/veo-adapter/server.js ./

RUN mkdir -p /data/tasks \
    && chown -R node:node /app /data/tasks

USER node
EXPOSE 8091
HEALTHCHECK --interval=15s --timeout=5s --start-period=5s --retries=4 \
  CMD node -e "fetch('http://127.0.0.1:8091/health').then(r=>{if(!r.ok)process.exit(1)}).catch(()=>process.exit(1))"

CMD ["node", "server.js"]
