# Anthropic to OpenAI Proxy for Claude Code

A lightweight Go proxy that translates Anthropic Messages API calls to OpenAI Chat Completions API. This allows you to use OpenAI-compatible backends (like local LLMs, LiteLLM, or OpenAI itself) with [Claude Code](https://code.claude.com/).

## Features

- Translates `/v1/messages` to `/v1/chat/completions`.
- Supports streaming (SSE) with Anthropic event mapping.
- Forwards necessary headers for Claude Code compatibility.
- Simple single-binary deployment.

## Usage

### 1. Build the proxy

```bash
go build -o anthropic-proxy
```

### 2. Run the proxy

Set the necessary environment variables:

```bash
export OPENAI_API_KEY=your-api-key
export OPENAI_BASE_URL=https://api.openai.com/v1 # Or your custom endpoint
export PORT=8080
./anthropic-proxy
```

### 3. Configure Claude Code

Set the `ANTHROPIC_BASE_URL` to point to your proxy:

```bash
export ANTHROPIC_BASE_URL=http://localhost:8080
export ANTHROPIC_API_KEY=any-value # Claude Code requires this to be set
```

If you are using it with LiteLLM or a local model, you might need to set the model name Claude Code uses:

```bash
# Example for a specific model
export CLAUDE_CODE_MODEL=gpt-4o
```

## How it works

The proxy implements the [Anthropic Messages API](https://docs.anthropic.com/claude/reference/messages_post) and maps:
- `system` prompt to a "system" message in OpenAI.
- `messages` array to OpenAI `messages`.
- `stream: true` to OpenAI streaming with event translation:
    - `message_start`
    - `content_block_start`
    - `content_block_delta`
    - `content_block_stop`
    - `message_delta`
    - `message_stop`

## Limitations

- Basic token counting (returns 0 for now).
- Simplified message content handling (concatenates text parts).
- Assumes the backend supports OpenAI-compatible Chat Completions.
