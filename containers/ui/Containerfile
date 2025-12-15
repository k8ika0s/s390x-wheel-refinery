FROM node:20-alpine AS build
WORKDIR /app
ARG VITE_API_BASE=http://localhost:8000
ENV VITE_API_BASE=${VITE_API_BASE}
COPY ui/package.json ui/vite.config.js ui/index.html /app/
COPY ui/src /app/src
COPY ui/src/styles.css /app/src/styles.css
RUN npm install
RUN npm run build

FROM nginx:alpine
COPY --from=build /app/dist /usr/share/nginx/html
EXPOSE 80
CMD ["nginx", "-g", "daemon off;"]
