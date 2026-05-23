# syntax=docker/dockerfile:1.7

# --- Build stage -------------------------------------------------------------
# node:22-alpine matches the @types/node ^24 dev dep loosely; node 22 is the
# current LTS and is what Vite 8 targets in CI.
FROM node:22-alpine AS builder

WORKDIR /app

# Copy the dependency manifest FIRST and install before copying any source.
# Docker layer caching means `npm ci` only re-runs when package.json or
# package-lock.json change — not on every source edit. --no-audit / --no-fund
# silence noise that adds seconds without changing the install result.
COPY package.json package-lock.json ./
RUN npm ci --no-audit --no-fund

# Now the source. Copy only what the build needs to keep the context tight.
COPY tsconfig.json tsconfig.app.json tsconfig.node.json ./
COPY vite.config.ts ./
COPY index.html ./
COPY public ./public
COPY src ./src

# `npm run build` = `tsc -b && vite build`. Output lands in /app/dist.
RUN npm run build

# --- Runtime stage -----------------------------------------------------------
# nginxinc/nginx-unprivileged runs as uid 101 (nginx) by default — no root,
# no setcap dance, no chowning the html dir at runtime.
FROM nginxinc/nginx-unprivileged:alpine

# Drop the stock /etc/nginx/conf.d/default.conf by replacing the whole nginx.conf
# (our config defines its own server block and worker settings).
COPY --chown=nginx:nginx nginx.conf /etc/nginx/nginx.conf

# Static assets.
COPY --from=builder --chown=nginx:nginx /app/dist /usr/share/nginx/html

EXPOSE 8080

# The base image's CMD already runs nginx in the foreground; leaving it as-is.
