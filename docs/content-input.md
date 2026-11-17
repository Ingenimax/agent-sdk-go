# Content input

You can send an agent more than text: for example, an image with a question
about it. The input is an `interfaces.Content`, an ordered list of parts that
follows the shape of Google ADK's `genai.Content`.

Each part sets one of the following fields:

- `Text`: plain text.
- `InlineData`: data sent with the request, as bytes plus a MIME type.
- `FileData`: data the provider fetches, as a URI plus a MIME type.

The MIME type says what the data is, such as `image/png` or
`application/pdf`. Build parts with `interfaces.NewPartFromText`,
`interfaces.NewPartFromBytes`, and `interfaces.NewPartFromURI`.

For a runnable example per provider, see
[examples/content](../examples/content).

## Send content to an agent

Pass the content to `RunWithContent`:

```go
png, err := os.ReadFile("chart.png")
if err != nil {
    return err
}

answer, err := myAgent.RunWithContent(ctx, interfaces.NewContentFromParts([]*interfaces.Part{
    interfaces.NewPartFromBytes(png, "image/png"),
    interfaces.NewPartFromText("What trend does this chart show?"),
}, interfaces.MessageRoleUser))
```

For a streamed answer, call `RunWithContentStream`. For token usage, call
`RunWithContentDetailed`. `Run(ctx, input)` is `RunWithContent` with one text
part, and it doesn't change.

The agent checks the content before the run starts. The role must be user or
empty, and each part must be valid. Input guardrails run on each text part.

When the agent has memory, it stores the content with its parts. Later turns
in the same conversation send the parts again, so the model can still refer
to them. Text-only runs store the same message they always have.

Custom run functions and remote agents take text only. For content with
media, they return an error that wraps `interfaces.ErrContentUnsupported`
instead of dropping the media.

## Send content to an LLM client

When you call a client directly without memory, pass the parts with
`interfaces.WithParts`. The parts replace the prompt, so include the text as a
text part:

```go
resp, err := client.Generate(ctx, "",
    interfaces.WithParts(
        interfaces.NewPartFromURI("https://example.com/photo.jpg", "image/jpeg"),
        interfaces.NewPartFromText("Describe this photo."),
    ),
)
```

With memory set, the client builds the request from memory and ignores
`WithParts`. Put the parts on the user message instead:

```go
err := mem.AddMessage(ctx, interfaces.Message{
    Role:    interfaces.MessageRoleUser,
    Content: "Describe this photo.",
    Parts: []*interfaces.Part{
        interfaces.NewPartFromURI("https://example.com/photo.jpg", "image/jpeg"),
        interfaces.NewPartFromText("Describe this photo."),
    },
})
```

## Send content from the agent CLI

In direct execution mode, `agent-cli` attaches images and PDFs to the prompt
with `--image-path`, `--image-url`, and `--pdf-path`. Each flag can be
repeated:

```bash
agent-cli --prompt "Does the chart match the numbers in the report?" \
  --image-path ./chart.png \
  --pdf-path ./report.pdf
```

For details, see the
[agent CLI README](../cmd/agent-cli/README.md#direct-execution-mode).

## Send content over HTTP

The run and stream endpoints of `microservice.NewHTTPServer` and
`microservice.NewHTTPServerWithUI`, `POST /api/v1/agent/run` and
`POST /api/v1/agent/stream`, take content as well as text. A request with
only `input` works as it always has.

With JSON, put the content in the `content` field, in the JSON form of
`interfaces.Content`. Inline data is base64. When `input` is also set, it is
sent after the content's parts, as a text part:

```bash
curl -s http://localhost:8080/api/v1/agent/run \
  -H 'Content-Type: application/json' \
  -d @- <<EOF
{
  "input": "What trend does this chart show?",
  "conversation_id": "conv-1",
  "content": {
    "parts": [
      {"inlineData": {"mimeType": "image/png", "data": "$(base64 < chart.png | tr -d '\n')"}}
    ]
  }
}
EOF
```

To send an image by URL instead, use a `fileData` part:

```json
{"fileData": {"mimeType": "image/jpeg", "fileUri": "https://example.com/photo.jpg"}}
```

With `multipart/form-data`, send the text in the `input` field and each file
in a `files` field. The `org_id` and `conversation_id` fields work as they do
in JSON. The files are sent first, in the order you upload them, followed by
`input`:

```bash
curl -s http://localhost:8080/api/v1/agent/run \
  --form-string 'input=Does the chart match the numbers in the report?' \
  -F files=@chart.png \
  -F files=@report.pdf
```

The stream endpoint takes the same bodies and answers with server-sent
events.

The server checks every request before the agent runs:

- The body can be up to 32 MiB (`microservice.MaxRunRequestBytes`), with
  files and base64 included.
- A request can carry up to 10 files or data parts
  (`microservice.MaxRunRequestFiles`).
- The server detects the type of each file and of inline data from the data,
  not from the file name or a declared `mimeType`, and accepts PNG, JPEG, GIF,
  and WebP images and PDFs. The detected type replaces a declared one.
- File data must declare one of those types and point at an `https` URL. The
  server refuses other schemes, such as `s3://` and `gs://`, because the
  provider would read them with the server's credentials.
- Each part must set `text`, `inlineData`, or `fileData`. A part with none of
  them, such as one with a misspelled `inline_data` field, is refused instead
  of being read as empty text.

The endpoints answer bad input with a 4xx status:

| Status | When |
| --- | --- |
| 400 | The JSON or form is invalid, there is no input, a file has a type the server doesn't accept, there are too many files, or a part is invalid. |
| 413 | The body is larger than 32 MiB. |
| 422 | The agent can't send a part, so it returns an error that wraps `interfaces.ErrContentUnsupported`. For example, Ollama takes no PDFs, and custom run functions and remote agents take text only. |

The stream endpoint returns the 422 before the SSE response starts. For a
request with files or data, it waits for the run's first event, because a
provider refuses a part before it sends anything else. Other agent errors are
reported as before: status 500 from the run endpoint of `NewHTTPServer`, an
`error` field with status 200 from the run endpoint of `NewHTTPServerWithUI`,
and an `error` event on the stream endpoints.

## Provider support

| Provider | What it sends |
| --- | --- |
| OpenAI | PNG, JPEG, WebP, and non-animated GIF images, inline (as a data URL) or by URL; PDFs, inline only |
| Azure OpenAI | PNG, JPEG, WebP, and GIF images, inline (as a data URL) or by URL |
| Anthropic | JPEG, PNG, GIF, and WebP images and PDFs, inline or by URL; inline only on Bedrock and Vertex AI |
| Gemini | Every part, as is; the API checks the MIME type and the URI |
| Bedrock (Converse) | Images, documents, video, and audio, inline or from an `s3://` URI |
| Ollama | Inline images |
| DeepSeek | JPEG, PNG, GIF, and WebP images, inline (as a data URL) or by URL, on `deepseek-flash` |
| vLLM | Images, inline (as a data URL) or by URL |

OpenAI sends a PDF's `DisplayName`, when you set it, as the file name:

```go
pdf := interfaces.NewPartFromBytes(data, "application/pdf")
pdf.InlineData.DisplayName = "report.pdf"
```

Gemini takes a URI that is a Files API URI, a `gs://` URI (Vertex AI only),
or a public HTTPS URL (not on Gemini 2.0 models).

Bedrock takes the following MIME types:

- Images: `image/png`, `image/jpeg`, `image/gif`, and `image/webp`.
- Documents: `application/pdf`, `text/csv`, `application/msword`,
  `application/vnd.openxmlformats-officedocument.wordprocessingml.document`,
  `application/vnd.ms-excel`,
  `application/vnd.openxmlformats-officedocument.spreadsheetml.sheet`,
  `text/html`, `text/plain`, and `text/markdown`.
- Video: `video/x-matroska`, `video/quicktime`, `video/mp4`, `video/webm`,
  `video/x-flv`, `video/mpeg`, `video/x-ms-wmv`, and `video/3gpp`.
- Audio: `audio/mpeg`, `audio/wav`, `audio/flac`, `audio/ogg`, `audio/aac`,
  `audio/opus`, `audio/mp4`, `audio/webm`, and `audio/x-matroska`.

Bedrock requires a unique name for each document, so set `DisplayName`.
Bedrock allows only ASCII letters and digits, hyphens, parentheses, square
brackets, and single spaces, up to 200 characters. The client turns every
other character into a space and cuts the name at 200 characters:
`report.pdf` is sent as `report pdf`. The client returns an error for a
document without a usable name, for two documents with the same name, and
for a message that has a document but no text part, because Bedrock rejects
each of these. Which kinds of media work depends on the model, and only
Amazon Nova models take `s3://` sources.

vLLM takes images only on its chat endpoint. When a `Generate` request has
images in any turn, the client uses the chat endpoint instead of the
completions endpoint and sends the user messages and assistant replies from
memory. It returns an error when you also set a `ResponseFormat`, because the
chat request doesn't apply it.

A provider that can't send a part returns an error that wraps
`interfaces.ErrContentUnsupported`. It doesn't drop the part and send the rest.

The model must also accept the media. A text-only model on a provider that
supports images returns the provider's own error.
