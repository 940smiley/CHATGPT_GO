# CHATGPT_GO

A Go-based API Gateway that automatically discovers local microservices (MCP Servers) and exposes them to a custom ChatGPT GPT via a single, dynamic OpenAPI schema.

## How it Works

1.  The application runs as a local server, acting as a proxy.
2.  It watches the `/mcp_servers` directory for new service configuration files.
3.  When a new service is detected, the gateway dynamically generates an `openapi.json` schema that includes the new service's endpoints.
4.  This gateway is exposed to the internet using a tunneling tool like `ngrok`.
5.  A single custom GPT is configured to use the gateway's `openapi.json` from the public `ngrok` URL.
6.  When an action is invoked in ChatGPT, the request hits the gateway, which then proxies it to the appropriate local MCP server.

This allows developers to add and remove local services that are accessible to ChatGPT without ever needing to reconfigure the GPT itself.

## Getting Started

### 1. Install Dependencies

Run the following command in the project root to download the necessary Go modules.
```sh
go mod tidy
```

### 2. Run the Example Services

Open two separate terminal windows to run the mock "MCP Servers".

**Terminal 1 (Weather Service):**
```sh
go run ./examples/weather_service/main.go
```

**Terminal 2 (Todo Service):**
```sh
go run ./examples/todo_service/main.go
```

### 3. Run the Gateway

In a third terminal, run the main gateway application.
```sh
go run main.go
```

### 4. Expose to the Internet

The gateway runs on `localhost:8080`. To make it accessible to ChatGPT, you need to expose it using a tunneling service like `ngrok`.

**Terminal 4 (ngrok):**
```sh
ngrok http 8080
```
`ngrok` will give you a public URL (e.g., `https://<random-id>.ngrok-free.app`).

### 5. Configure ChatGPT

1.  Create a new GPT in ChatGPT.
2.  Go to **Configure -> Actions -> Create new action**.
3.  Select **Import from URL**.
4.  Paste the URL to your gateway's OpenAPI schema: `https://<random-id>.ngrok-free.app/openapi.json`.
5.  Follow the prompts to import the actions.

You can now chat with your GPT and invoke actions like "What is the weather in London?" or "What's on my todo list?". If you add a new `.yaml` file to the `mcp_servers` directory, the gateway will automatically pick it up!
