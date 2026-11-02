FROM node:24-alpine AS build
WORKDIR /src
COPY package.json package-lock.json ./
RUN npm ci
COPY . .
# Chỉ đặt khi console và API khác origin; để trống thì nginx proxy /api về backend.
ARG VITE_API_BASE_URL=
ENV VITE_API_BASE_URL=${VITE_API_BASE_URL}
RUN npm run build

FROM nginxinc/nginx-unprivileged:1.29-alpine AS console
# Image gốc render *.template bằng envsubst lúc khởi động, chỉ thay biến đã định nghĩa.
COPY nginx.conf.template /etc/nginx/templates/default.conf.template
COPY --from=build /src/dist /usr/share/nginx/html
ENV TRACEPILOT_API_UPSTREAM=http://tracepilot:8080
EXPOSE 8080
USER nginx
HEALTHCHECK --interval=10s --timeout=3s --retries=3 CMD ["wget", "-q", "-O", "/dev/null", "http://127.0.0.1:8080/healthz"]
