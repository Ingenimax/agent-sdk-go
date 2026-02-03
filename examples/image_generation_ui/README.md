# Image Generation UI Example (GCS Storage)

This example demonstrates image generation capabilities with the embedded web UI using Google Cloud Storage. The agent can generate images based on text descriptions and display them in the chat interface.

## Features

- **Image Generation**: Generate images using Gemini's image generation models
- **GCS Storage**: Images are stored in Google Cloud Storage with public URLs
- **Auto Bucket Creation**: The bucket is created automatically if it doesn't exist
- **Embedded Web UI**: Interactive chat interface with image display
- **Image Lightbox**: Click on generated images to view them full-size
- **Download Support**: Download generated images directly from the UI
- **Fallback**: If GCS is not configured, falls back to base64 data URIs

## Prerequisites

### 1. Gemini API Credentials

You need one of the following authentication methods:

**Option A: Gemini API Key**
```bash
export GEMINI_API_KEY="your-api-key"
```

**Option B: Vertex AI (Google Cloud)**
```bash
export VERTEX_AI_PROJECT="your-project-id"
export VERTEX_AI_REGION="us-central1"
export VERTEX_AI_GOOGLE_APPLICATION_CREDENTIALS_CONTENT="base64-encoded-credentials"
```

### 2. GCS Storage (Recommended)

For images to display properly in the browser, configure GCS storage:

```bash
# Required: Bucket name (will be created if it doesn't exist)
export GCS_BUCKET="your-bucket-name"

# Required for bucket creation: GCP Project ID
export GCS_PROJECT="your-gcp-project-id"

# Optional: Prefix for organizing images (default: "generated-images")
export GCS_PREFIX="my-images"

# Authentication: Use one of these methods
# Option 1: Service account JSON file
export GOOGLE_APPLICATION_CREDENTIALS="/path/to/service-account.json"

# Option 2: Application Default Credentials (if running on GCP)
# No environment variable needed
```

### 3. GCP Permissions

The service account needs the following permissions:
- `storage.buckets.create` (for auto bucket creation)
- `storage.buckets.get`
- `storage.objects.create`
- `storage.objects.get`
- `resourcemanager.projects.get` (optional, for bucket creation)

Or use the predefined role: `roles/storage.admin`

## Running the Example

```bash
cd examples/image_generation_ui
go run main.go
```

Then open your browser to: http://localhost:8080

## Usage

Once the UI is open, try asking:

- "Generate an image of a sunset over mountains"
- "Create a minimalist logo for a tech company"
- "Draw a cute cartoon cat"
- "Make a futuristic cityscape at night"

The agent will use the image generation tool and display the result in the chat. Click on any generated image to view it in full size.

## How It Works

1. The agent uses Gemini's `gemini-2.5-flash-image` model for image generation
2. A separate `gemini-2.5-flash` model handles conversation and decides when to generate images
3. Generated images are uploaded to GCS and served via public URLs
4. The UI renders images using markdown image syntax
5. If GCS is not configured, images are embedded as base64 data URIs (larger payloads)

## Architecture

```
┌─────────────────────────────────────────┐
│           Web Browser (UI)              │
│  ┌─────────────────────────────────┐   │
│  │    Chat Interface               │   │
│  │    ┌───────────────────────┐    │   │
│  │    │  Generated Image      │    │   │
│  │    │  (from GCS URL)       │    │   │
│  │    └───────────────────────┘    │   │
│  └─────────────────────────────────┘   │
└────────────────┬────────────────────────┘
                 │ HTTP/SSE
                 ▼
┌─────────────────────────────────────────┐
│         Microservice Server             │
│  ┌─────────────────────────────────┐   │
│  │          Agent                  │   │
│  │  ┌─────────────┐ ┌───────────┐ │   │
│  │  │ Text LLM    │ │ Image Gen │ │   │
│  │  │ (Gemini)    │ │   Tool    │ │   │
│  │  └─────────────┘ └─────┬─────┘ │   │
│  └────────────────────────┼────────┘   │
│                           │            │
│  ┌────────────────────────▼────────┐   │
│  │       GCS Storage               │   │
│  │   gs://bucket/generated-images/ │   │
│  └─────────────────────────────────┘   │
└─────────────────────────────────────────┘
                 │
                 │ Public URL
                 ▼
┌─────────────────────────────────────────┐
│    Google Cloud Storage (GCS)           │
│    https://storage.googleapis.com/...   │
└─────────────────────────────────────────┘
```

## Configuration Options

### Image Generation Tool

```go
imgTool := imagegen.New(imageClient, imgStorage,
    imagegen.WithMaxPromptLength(2000),      // Max prompt length
    imagegen.WithDefaultAspectRatio("16:9"), // Default aspect ratio
    imagegen.WithDefaultFormat("jpeg"),      // Default output format
)
```

### Supported Aspect Ratios

- `1:1` (square, default)
- `16:9` (widescreen)
- `9:16` (portrait)
- `4:3` (standard)
- `3:4` (portrait)

### Supported Output Formats

- `png` (default)
- `jpeg`

## Troubleshooting

### Images not displaying

1. **Check GCS bucket permissions**: The bucket needs public read access for `allUsers`
2. **Check CORS**: If using a custom domain, configure CORS on the bucket
3. **Check logs**: The tool logs the generated URL - verify it's accessible

### Bucket creation fails

1. **Check project ID**: Ensure `GCS_PROJECT` is set correctly
2. **Check permissions**: Service account needs `storage.buckets.create`
3. **Check billing**: GCS requires a billing-enabled project

### Falling back to base64

If you see "using base64 fallback" in the logs:
1. Check `GCS_BUCKET` is set
2. Check GCP credentials are configured
3. Check network connectivity to GCS

## Environment Variables Summary

| Variable | Required | Description |
|----------|----------|-------------|
| `GEMINI_API_KEY` | Yes* | Gemini API key for image generation |
| `VERTEX_AI_PROJECT` | Yes* | Vertex AI project ID |
| `VERTEX_AI_REGION` | No | Vertex AI region (default: us-central1) |
| `VERTEX_AI_GOOGLE_APPLICATION_CREDENTIALS_CONTENT` | Yes* | Base64-encoded service account JSON |
| `GCS_BUCKET` | No | GCS bucket name for image storage |
| `GCS_PROJECT` | No | GCP project ID (for bucket creation) |
| `GCS_PREFIX` | No | Path prefix in bucket (default: generated-images) |
| `GOOGLE_APPLICATION_CREDENTIALS` | No | Path to service account JSON |

*Either `GEMINI_API_KEY` or Vertex AI credentials are required.
