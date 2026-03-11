## UI Multimodal Server (Example)

This example starts an HTTP server with an **embedded Web UI** (`HTTPServerWithUI`) and demonstrates how to send an image using **multimodal `content_parts`** (the image is sent as an `image_url` data URL: `data:image/*;base64,...`) via the browser UI.

### 1) Start the server (UI + API)

Run from the repository root:

```bash
# Optional: create a .env file under this example directory
# examples/microservices/ui_multimodal_server/.env
# OPENAI_API_KEY=...
# OPENAI_MODEL=...
# OPENAI_BASE_URL=...   # optional OpenAI-compatible gateway

go run ./examples/microservices/ui_multimodal_server
```

Default port is `8085`. You can override it via `UI_PORT` (either in `.env` or as an environment variable).

### 2) Use the browser UI

Open:

- `http://localhost:8085/`

Attach an image in the chat input area and send a prompt. The UI will send the image via `content_parts`.

