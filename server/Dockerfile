FROM node:24-alpine

RUN apk add --no-cache git

WORKDIR /app
COPY package.json ./
COPY src ./src

ENV PORT=3000
ENV SNIPPET_REPOSITORIES_PATH=/repositories
ENV GIT_CONFIG_COUNT=1
ENV GIT_CONFIG_KEY_0=safe.directory
ENV GIT_CONFIG_VALUE_0=*

EXPOSE 3000
CMD ["node", "src/server.mjs"]
