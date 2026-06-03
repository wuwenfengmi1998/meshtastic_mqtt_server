# MeshMap Frontend

Vue 3 + TypeScript + Vite frontend for the Meshtastic MQTT server.

## Features

- Left panel: recent chat messages
- Right panel: Leaflet/OpenStreetMap node map
- Bottom panel: selected node details, recent messages, and recent positions

The app uses relative `/api` URLs. In development, Vite proxies `/api` to the Go backend.

## Development

Start the Go backend:

```bash
go run . --web-host 127.0.0.1 --web-port 8080
```

Start the frontend dev server:

```bash
cd meshmap_frontend
npm install
npm run dev
```

## Production build

```bash
cd meshmap_frontend
npm run build
```

The build output is written to the repository root `dist/` directory, which is served by the Gin backend.

## Map tiles

The map uses Leaflet with OpenStreetMap tiles:

```text
https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png
```

Network access to the tile server is required unless this is changed to a local tile source later.
