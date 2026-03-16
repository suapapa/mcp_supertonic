# Agent Guidelines for Supertonic TTS

This file documents building guidelines and rules for AI assistants or agents operating in this workspace.

---

## 🛑 Critical Rules for Maintenance

-   **Standard I/O Silence (CRITICAL)**: Any edits to files inside `internal/tts/supertonic/` must NOT print to `Stdout` using `fmt.Printf`, `fmt.Println`, or similar statements. Doing so will corrupt the JSON-RPC stream used for building the Stdio transport over MCP and crash the server stream. Use `fmt.Fprintf(os.Stderr, ...)` for debug triggers.
-   **Style Caching**: Load style weights through caching mechanisms to avoid CPU reloading penalties across multiple tools callbacks.

---

## 🛠 Workspace Layout

All components inside this server are managed using building blocks in Go:

### Go Setup
```go
package main
```

### Main Server (`./main.go`)
Sets up the `server.MCPServer` instance and registers the `synthesize_speech` tool. Holds standard style mappings.

### Extensions (`internal/tts/supertonic/tts_extensions.go`)
Allows extensions regarding returning duration in seconds or passing cached states to model execution steps.

---

## 💡 Code Expansion Guidelines

-   If appending new configuration variables for model speed or silence duration, export them in custom parameter maps.
-   Run `go build -o /tmp/mcp-supertonic .` to verify changes compile properly before submitting codebases to client review.
-   Any test files should be run isolated like `test_cmd`, and ignored during standard production compilations.