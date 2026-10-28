# ---- build stage: compile the SvelteKit app to static files ----
# SvelteKit 3 needs Node 22.17 or newer; the node:22 tag always points at the latest 22.x.
FROM node:22-alpine AS build
WORKDIR /app
COPY package.json package-lock.json ./
RUN npm ci
COPY . .
RUN npm run build

# ---- runtime stage: nginx serves the files and proxies /api to the Go service ----
FROM nginx:1.27-alpine
COPY nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /app/build /usr/share/nginx/html
EXPOSE 80
