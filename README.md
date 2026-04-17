# Supertonic TTS MCP Server

An MCP (Model Context Protocol) server for Supertonic Text-to-Speech (TTS) engine, allowing AI assistants to generate high-quality voice synthetic speech directly into audio files on disk.

## Features

-   **`synthesize_speech` tool**: Convert text into `.wav` audio using available voice nodes.
-   **`batch_synthesize_speech` tool**: Convert text into multiple audio files with variations in a batch for candidate selection.
-   **Voice Styles**: Supports fully descriptive pre-trained voices like F1, M2 backends.
-   **Cached Loading**: Optimizes weight reloading overhead on repeated synthesis requests.

## Prerequisites

To run the Supertonic TTS engine, you must download the pre-trained model files (ONNX nodes) to `~/.local/share/supertonic2`. This can be done automatically using Go generate:

```bash
go generate
```

## Available Voice Presets
Supported pre-defined voices under the `voice_styles` directory:
-   `F1`, `F2`, `F3`, `F4`, `F5` (Female)
-   `M1`, `M2`, `M3`, `M4`, `M5` (Male)

---

## Installation & Running

### 1. Install
Compile and install the application using Go:

```bash
go install
```

> [!NOTE]
> This places the `mcp_supertonic` binary into your `~/go/bin` directory. Ensure this folder is in your system `PATH`.

### 2. Running

#### Standard I/O Mode (Default)
Run the binary normally. It operates in the Standard I/O mode, perfect for adding to local AI clients:

```bash
mcp_supertonic
```

#### SSE Network Mode (HTTP Server)
If you want to deploy the server on a local or external network, you can start an SSE HTTP server by providing a port number:

```bash
mcp_supertonic -port 8080
```
This mode exposes the MCP over an SSE stream on `http://<IP_ADDRESS>:8080/sse` and the message endpoint on `/message`.

### Client Configuration

#### For Standard I/O (Local Client)
To use this server locally with a client like **Claude Desktop** or **Cursor**, add the following snippet to your `mcpServers` configuration file (e.g., `claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "supertonic-tts": {
      "command": "mcp_supertonic",
      "env": {
        "ONNXRUNTIME_LIB_PATH": "/opt/homebrew/opt/onnxruntime/lib/libonnxruntime.dylib"
      }
    }
  }
}
```

> [!TIP]
> Make sure `mcp_supertonic` is in your `PATH` (typically `~/go/bin`), or provide the absolute path to the binary.

#### For SSE Network (Remote Client)
For clients that support HTTP/SSE MCP connections, you typically configure them with the SSE URL instead of a command.

Example configuration snippet:
```json
{
  "mcpServers": {
    "supertonic-tts-remote": {
      "type": "sse",
      "url": "http://<SERVER_IP>:8080/sse"
    }
  }
}
```
Replace `<SERVER_IP>` with the IP address of the machine running `mcp_supertonic`.

---

## MCP Tool Parameters

### `synthesize_speech`

| Argument | Type | Description | Default |
| :--- | :--- | :--- | :--- |
| **`input_text`** (Required) | `string` | Text payload to synthesize. | - |
| `output_filename` | `string` | Filepath or name of the output `.wav` file. | `speech.wav` |
| `voice` | `string` | Preset name such as `F1` or `M2`. | `F1` |
| `lang` | `string` | Language code (`ko`, `en`, `es`, `pt`, `fr`). Leaves blank to auto-detect. | `""` (Auto) |
| `speed` | `number` | Speed rate multiplier to synthesize speech (e.g., `1.0`). | `1.0` |

---

### `batch_synthesize_speech`

| Argument | Type | Description | Default |
| :--- | :--- | :--- | :--- |
| **`input_text`** (Required) | `string` | Text payload to synthesize. | - |
| `batch_cnt` | `number` | Number of variations to generate (audio count). | `3` |
| `voice` | `string` | Preset name such as `F1` or `M2`. | `F1` |
| `lang` | `string` | Language code (`ko`, `en`, `es`, `pt`, `fr`). Leaves blank to auto-detect. | `""` (Auto) |
| `speed` | `number` | Speed rate multiplier to synthesize speech (e.g., `1.0`). | `1.0` |
| `output_dir` | `string` | Directory path to save output WAV files. | `.` |

---

---

## Usage Examples

Here are four examples of how to call the tools.

### 1. Basic Korean Synthesis (Default Voice)
Synthesize Korean text using the default female voice (`F1`).

**Arguments:**
```json
{
  "input_text": "안녕하세요. Supertonic TTS 엔진을 테스트 중입니다.",
  "output_filename": "hello_ko.wav"
}
```

**Response:**
```json
{
  "audio_saved_to": "/Users/suapapa/ws_suapapa/mcp_supertonic/hello_ko.wav",
  "duration_seconds": 3.52
}
```

### 2. English Synthesis with Speed Control
Synthesize English text using a male voice (`M1`) at a slightly faster speed (`1.2x`).

**Arguments:**
```json
{
  "input_text": "Hello! This is a test of the Supertonic Text-to-Speech system.",
  "voice": "M1",
  "speed": 1.2,
  "output_filename": "hello_en.wav"
}
```

**Response:**
```json
{
  "audio_saved_to": "/Users/suapapa/ws_suapapa/mcp_supertonic/hello_en.wav",
  "duration_seconds": 4.15
}
```

### 3. Multi-language Auto-detection
Synthesize Spanish text with automatic language detection using voice `F3`.

**Arguments:**
```json
{
  "input_text": "Hola, amigos. Bienvenidos a Supertonic.",
  "voice": "F3",
  "lang": "",
  "output_filename": "hola_es.wav"
}
```

**Response:**
```json
{
  "audio_saved_to": "/Users/suapapa/ws_suapapa/mcp_supertonic/hola_es.wav",
  "duration_seconds": 2.80
}
```


### 4. Batch Synthesis with Variations
Generate multiple candidates of the same text with slight variations. The filenames are automatically generated from the input text (max 20 chars slug).

**Arguments:**
```json
{
  "input_text": "안녕하세요. 일괄 생성된 오디오입니다.",
  "batch_cnt": 3,
  "output_dir": "outputs"
}
```

**Response:**
```json
{
  "saved_files": [
    {
      "path": "/Users/suapapa/ws_suapapa/mcp_supertonic/안녕하세요_1.wav",
      "duration_seconds": 2.45
    },
    {
      "path": "/Users/suapapa/ws_suapapa/mcp_supertonic/안녕하세요_2.wav",
      "duration_seconds": 2.41
    },
    {
      "path": "/Users/suapapa/ws_suapapa/mcp_supertonic/안녕하세요_3.wav",
      "duration_seconds": 2.48
    }
  ]
}
```

---

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

This project relies on [Supertonic-2](https://huggingface.co/Supertone/supertonic-2) and uses standard ONNX runtimes.
