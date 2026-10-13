# Versions are pinned so builds are repeatable; see README (Versions).
FROM node:20.18.0 AS build-frontend

WORKDIR /app

COPY package.json package-lock.json ./

COPY .env.production ./
COPY env.d.ts ./
COPY postcss.config.js ./
COPY tailwind.config.js ./
COPY tsconfig.app.json ./
COPY tsconfig.json ./
COPY tsconfig.node.json ./
COPY vite.config.ts ./
COPY index.html ./

COPY public/ public/
COPY src/ src/

RUN npm ci --no-audit --no-fund

RUN npm run build

FROM caddy:2.8.4

COPY --from=build-frontend /app/dist/ /dist/