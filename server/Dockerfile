FROM node:20-alpine AS build

WORKDIR /app

RUN corepack enable

COPY package.json pnpm-lock.yaml .npmrc ./
COPY tsconfig.json nest-cli.json ./
COPY prisma ./prisma
COPY src ./src

RUN pnpm install --frozen-lockfile
RUN pnpm prisma:generate
RUN pnpm build
RUN pnpm prune --prod

FROM node:20-alpine AS runtime

WORKDIR /app

ENV NODE_ENV=production

COPY --from=build /app/package.json ./package.json
COPY --from=build /app/node_modules ./node_modules
COPY --from=build /app/prisma ./prisma
COPY --from=build /app/dist ./dist

EXPOSE 3000

CMD ["node", "dist/main.js"]
