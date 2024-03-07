FROM node:18 AS build-frontend

WORKDIR /app

COPY package.json ./

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

RUN npm install

RUN npm run build

FROM caddy:2

COPY --from=build-frontend /app/dist/ /dist/