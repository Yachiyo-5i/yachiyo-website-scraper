FROM mcr.microsoft.com/playwright:v1.61.0-noble

ENV NODE_ENV=production \
    HOME=/home/pwuser \
    PLAYWRIGHT_BROWSERS_PATH=/ms-playwright \
    PORT=3001

WORKDIR /app

COPY package.json ./
RUN npm install --omit=dev --no-audit --no-fund

COPY server.mjs ./
RUN chown -R pwuser:pwuser /app /home/pwuser

USER pwuser

EXPOSE 3001

HEALTHCHECK --interval=30s --timeout=5s --start-period=20s --retries=3 \
  CMD node -e "fetch('http://127.0.0.1:3001/health').then(r=>process.exit(r.ok?0:1)).catch(()=>process.exit(1))"

CMD ["npm", "start"]
