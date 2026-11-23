# syntax=docker/dockerfile:1
FROM --platform=$BUILDPLATFORM node:24-alpine AS build
WORKDIR /src
COPY package.json package-lock.json ./
RUN --mount=type=cache,target=/root/.npm npm ci --no-audit --no-fund
COPY index.html tsconfig.json vite.config.ts ./
COPY src ./src
COPY public ./public
ARG VITE_API_BASE_URL=
ENV VITE_API_BASE_URL=${VITE_API_BASE_URL}
RUN npm run build

FROM nginxinc/nginx-unprivileged:1.29-alpine AS console
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_TIME=unknown
LABEL org.opencontainers.image.title="TracePilot Console" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.revision="${COMMIT}" \
      org.opencontainers.image.created="${BUILD_TIME}"
COPY nginx.conf.template /etc/nginx/templates/default.conf.template
COPY --from=build /src/dist /usr/share/nginx/html
ENV TRACEPILOT_API_UPSTREAM=http://tracepilot:8080
ENV NGINX_ENVSUBST_FILTER=^TRACEPILOT_API_UPSTREAM$
EXPOSE 8080
USER nginx
HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --start-interval=3s --retries=3 CMD ["wget", "-q", "-O", "/dev/null", "http://127.0.0.1:8080/healthz"]
