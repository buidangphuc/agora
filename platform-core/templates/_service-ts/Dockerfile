# Multi-stage build for the TS edge (BFF). Non-root, minimal runtime image.

# ── deps + build ────────────────────────────────────────────────────────────
FROM node:20-alpine AS build
WORKDIR /app

# Install all deps (incl. dev) for the build/proto step.
COPY package.json package-lock.json* ./
RUN npm ci || npm install

# Bring in source + config. NOTE: `generated/` must exist at build time —
# generate it first (`npm run proto` with proto-vendor/ checked out) or add a
# proto-generation stage before this one. It is gitignored, so it is NOT copied
# from a fresh checkout by default.
COPY . .
RUN npm run build

# Prune to production dependencies only.
RUN npm prune --omit=dev

# ── runtime ─────────────────────────────────────────────────────────────────
FROM node:20-alpine AS runtime
WORKDIR /app
ENV NODE_ENV=production

# node:alpine ships a non-root `node` user (uid 1000).
COPY --from=build --chown=node:node /app/node_modules ./node_modules
COPY --from=build --chown=node:node /app/dist ./dist
COPY --from=build --chown=node:node /app/package.json ./package.json

USER node

# Matches PORT in config.ts / .env.example. Override PORT at runtime as needed.
ENV PORT=3000
EXPOSE 3000

CMD ["node", "dist/server.js"]
