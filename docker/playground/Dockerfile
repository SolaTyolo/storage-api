ARG TARGETPLATFORM=linux/amd64
ARG NODE_IMAGE=node:20-alpine
ARG NGINX_IMAGE=nginx:alpine

FROM --platform=${TARGETPLATFORM} ${NODE_IMAGE} AS build
WORKDIR /app

ARG VITE_GATEWAY_ENDPOINT=http://localhost:8080/api/v1
ARG VITE_DEFAULT_BUCKET=uploads
ARG VITE_GATEWAY_TOKEN
ENV VITE_GATEWAY_ENDPOINT=$VITE_GATEWAY_ENDPOINT \
    VITE_DEFAULT_BUCKET=$VITE_DEFAULT_BUCKET \
    VITE_GATEWAY_TOKEN=$VITE_GATEWAY_TOKEN

COPY packages/sdk/package.json packages/sdk/package-lock.json ./packages/sdk/
COPY packages/sdk/tsconfig.json packages/sdk/vitest.config.ts ./packages/sdk/
COPY packages/sdk/src ./packages/sdk/src
RUN cd packages/sdk && npm ci && npm run build

COPY packages/playground/package.json ./packages/playground/
COPY packages/playground/tsconfig.json packages/playground/vite.config.ts packages/playground/index.html ./packages/playground/
COPY packages/playground/src ./packages/playground/src
WORKDIR /app/packages/playground
RUN npm install && npm run build

FROM --platform=${TARGETPLATFORM} ${NGINX_IMAGE}
COPY docker/playground/nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /app/packages/playground/dist /usr/share/nginx/html
EXPOSE 80
