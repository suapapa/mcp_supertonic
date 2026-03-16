# Supertonic TTS MCP Server

An MCP (Model Context Protocol) server for Supertonic Text-to-Speech (TTS) engine, allowing AI assistants to generate high-quality voice synthetic speech directly into audio files on disk.

## Features

-   **`synthesize_speech` tool**: Convert text into `.wav` audio using available voice nodes.
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
Run the binary normally. It operates in the Standard I/O mode, perfect for adding to local AI clients:

```bash
mcp_supertonic
```

### Client Configuration

To use this server with a client like **Claude Desktop** or **Cursor**, add the following snippet to your `mcpServers` configuration file (e.g., `claude_desktop_config.json`):

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

## Usage Examples

Here are three examples of how to call the `synthesize_speech` tool.

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


---

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

This project relies on [Supertonic-2](https://huggingface.co/Supertone/supertonic-2) and uses standard ONNX runtimes.
