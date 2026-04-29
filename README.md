# Proxy-AI: Anthropic to OpenAI Bridge

Proxy-AI is a high-performance, lightweight gateway designed to bridge the gap between tools built for the **Anthropic Messages API** (such as the [Claude Code](https://code.claude.com/) CLI) and **OpenAI-compatible backends**. 

It allows you to use local LLMs (Ollama, vLLM), specialized providers (DeepSeek, OpenCode, NVIDIA NIM), or any OpenAI-compliant API while maintaining the rich features of Anthropic-native clients, including streaming and tool use.

## Key Features

- **Protocol Translation**: Seamlessly maps Anthropic `/v1/messages` to OpenAI `/v1/chat/completions`.
- **Smart Streaming**: Full support for Server-Sent Events (SSE) with intelligent mapping of "thinking" or reasoning blocks into Anthropic's native `thinking` blocks.
- **Tool Use Support**: Bidirectional translation between Anthropic's tool schema and OpenAI's function calling.
- **Profile Management**: Multi-backend support with dedicated configurations for model mapping, base URLs, and authentication.
- **Integrated Launcher**: Acts as a wrapper for the `claude` CLI, automatically configuring the environment for zero-config operation.
- **Web-Based Config UI**: A built-in dashboard to manage profiles, models, and keys without manual YAML editing.

## Installation & Building

Ensure you have [Go](https://go.dev/) 1.21+ installed.

```bash
# Clone the repository
git clone https://github.com/your-username/anthropic-proxy.git
cd anthropic-proxy

# Build using Makefile
make build
```

The binary will be available at `bin/proxy-ai`.

## Configuration

Proxy-AI manages its configuration at `~/.proxy-ai/config.yaml`. A default config is created on the first run.

### Profile Structure

```yaml
port: "8080"
default_profile: "my-backend"
profiles:
  my-backend:
    openai_base_url: "https://api.example.com/v1"
    
    # Authentication (choose one)
    openai_api_key: "sk-..."          # Plain text key
    openai_api_key_env: "MY_API_KEY"  # Load from environment variable
    
    # Map Anthropic models requested by the client to backend models
    model_mapping:
      claude-3-7-sonnet-20250219: "gpt-4o"
      claude-3-5-haiku-20241022: "gpt-4o-mini"
```

## Usage

Proxy-AI is designed to be zero-config after your initial profile setup in `~/.proxy-ai/config.yaml`.

### 1. Launcher Mode (Claude Code)
This is the preferred way to use Proxy-AI with Claude Code. The proxy starts the `claude` process and automatically injects the necessary environment variables (`ANTHROPIC_BASE_URL`, `ANTHROPIC_MODEL`, etc.) based on your profile.

```bash
# Uses the 'default_profile' defined in your config
./bin/proxy-ai

# Use a specific profile (e.g., nvidia)
./bin/proxy-ai --profile nvidia

# Enable verbose logging to see the proxy translation in real-time
./bin/proxy-ai --verbose
```

### 2. Serve Mode
Run as a standalone proxy server. This is useful if you want to point other clients (like a custom UI or a different CLI) to the proxy.

```bash
./bin/proxy-ai serve
```

The server will listen on the port defined in your config (default `8080`).

- **Dashboard**: `http://localhost:8080/` - Access the Web UI to manage configurations.
- **Default Route**: `http://localhost:8080/v1/messages` (uses `default_profile`)
- **Profile Route**: `http://localhost:8080/{profile_name}/v1/messages` (uses `{profile_name}`)

## Project Structure

- `cmd/proxy-ai`: Application entry point and CLI logic.
- `internal/proxy`: Core proxy engine, including request handling and SSE streaming.
- `internal/translator`: Transformation logic for messages, tools, and content blocks.
- `internal/config`: Profile and configuration management.
- `internal/types`: Type definitions for Anthropic and OpenAI schemas.

## License

MIT License. See [LICENSE](LICENSE) for details.
