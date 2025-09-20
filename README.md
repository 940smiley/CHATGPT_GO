# CHATGPT_GO

A Go-based gateway that discovers locally installed MCP (Model Context Protocol) servers and exposes them to a single custom ChatGPT GPT through an automatically generated OpenAPI schema. Drop a YAML file describing a local service into the `mcp_servers` directory and the gateway will immediately proxy requests to it and update the schema imported by your GPT.

## Features

- ⚙️ **Automatic discovery** – watches a directory for new/updated YAML service definitions.
- 📄 **Dynamic OpenAPI generation** – serves a consolidated `openapi.json` that always reflects the active MCP servers.
- 🔁 **Reverse proxy** – forwards action requests from ChatGPT to the correct local service, preserving headers and query parameters.
- 🏷️ **Service metadata** – annotates each operation with vendor extensions (`x-service-name`, `x-service-address`) so you can trace requests back to their source.
- 🛡️ **CORS friendly** – responds to `OPTIONS` requests and allows cross-origin calls, making tunnelling tools like ngrok painless.

## Getting Started

### Prerequisites

- Go 1.21+
- (Optional) [ngrok](https://ngrok.com/) or another tunnelling utility to expose the gateway to the internet.

### 1. Fetch dependencies

```bash
go mod tidy
```

### 2. Start the example MCP servers (optional but recommended)

These mock services live under `examples/` and mirror the sample YAML definitions.

```bash
# Terminal 1 – Weather service
cd examples/weather_service
go run .
```

```bash
# Terminal 2 – Todo service
cd examples/todo_service
go run .
```

Both services listen on `localhost` ports (`9001` and `9002`). Feel free to swap in your own MCP servers using the same addresses.

### 3. Run the gateway

```bash
go run .
```

By default the gateway listens on `:8080` and watches the `./mcp_servers` directory. The first run ships with two sample YAML files:

- `mcp_servers/weather.yaml`
- `mcp_servers/todo.yaml`

Add, edit, or remove YAML files in that directory and the gateway will reload automatically—no restart required.

### 4. (Optional) Expose the gateway to ChatGPT

Use a tunnelling service to make the local server reachable from ChatGPT. For example with ngrok:

```bash
ngrok http 8080
```

Take note of the public URL (e.g. `https://<random-id>.ngrok-free.app`).

### 5. Configure your custom GPT

1. Create or edit a GPT at [chat.openai.com](https://chat.openai.com/).
2. Open **Configure → Actions** and choose **Import from URL**.
3. Paste the gateway schema URL: `https://<ngrok-id>.ngrok-free.app/openapi.json`.
4. Save the GPT. Actions are now routed to whatever MCP servers are described in `mcp_servers/`.

## Defining Services

Service definitions are plain YAML. Each file represents one MCP server and can contain multiple endpoints. Example (`mcp_servers/weather.yaml`):

```yaml
serviceName: weather
serviceAddress: http://localhost:9001
description: "A service that returns the current weather for a city."
endpoints:
  - path: /weather/{city}
    method: GET
    description: "Get the current weather for a specific city."
    operationId: getWeatherForCity
    parameters:
      - name: city
        in: path
        description: "City to look up."
        schema:
          type: string
```

Endpoints support:

- Path, method, description, and optional `operationId`.
- Path and query parameters (path parameters are auto-marked as required).
- Optional request bodies with arbitrary JSON schema snippets.

Add as many YAML files as you need; the gateway merges them into a single OpenAPI document, tagging each operation with the originating service.

## Configuration Reference

| Option | Description | Default |
| ------ | ----------- | ------- |
| `CHATGPT_GATEWAY_CONFIG` | Directory to watch for YAML files. | `./mcp_servers` |
| `CHATGPT_GATEWAY_ADDR` | Exact address (`host:port`) for the HTTP server. | *(unset)* |
| `CHATGPT_GATEWAY_PORT` | Port (or `host:port`) if `CHATGPT_GATEWAY_ADDR` is unset. | `8080` |
| `--config` | CLI flag alternative to `CHATGPT_GATEWAY_CONFIG`. | `./mcp_servers` |
| `--addr` | CLI flag alternative to `CHATGPT_GATEWAY_ADDR`. | `:8080` |

CLI flags override environment variables.

## How the Gateway Works

1. Service YAML files are parsed into in-memory definitions.
2. The gateway builds a route table (method + path → service).
3. On every HTTP request (except `/openapi.json`), it finds the matching route and proxies the call to the service's `serviceAddress`.
4. The `/openapi.json` endpoint returns a merged OpenAPI 3.1 schema that ChatGPT uses for action discovery.

Any file creation, modification, removal, or rename inside the config directory triggers a reload and schema regeneration.

## Development Tips

- Enable verbose logging by running the gateway directly (`go run .`)—you'll see proxy activity and file watcher events.
- Want to model a new MCP server? Copy one of the sample YAML files and adjust the metadata, endpoints, and address.
- You can inspect the generated OpenAPI document locally at [http://localhost:8080/openapi.json](http://localhost:8080/openapi.json).
- The helper services under `examples/` are intentionally simple and stateless, making them easy to adapt or replace.

Happy hacking! Drop your services in `mcp_servers/` and they instantly become available to your GPT under Developer Mode.
