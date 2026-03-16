package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/suapapa/mcp_supertonic/internal/tts/supertonic"
)

var (
	styleCache = make(map[string]*supertonic.Style)
	cacheMu    sync.Mutex
)

func getStyle(voice string, voiceDir string) (*supertonic.Style, error) {
	cacheMu.Lock()
	defer cacheMu.Unlock()

	if style, ok := styleCache[voice]; ok {
		return style, nil
	}

	voiceFile := voice
	if !strings.HasSuffix(voiceFile, ".json") {
		voiceFile = voiceFile + ".json"
	}
	stylePath := filepath.Join(voiceDir, voiceFile)

	// Check if file exists
	if _, err := os.Stat(stylePath); err != nil {
		return nil, fmt.Errorf("voice style file not found: %s", stylePath)
	}

	style, err := supertonic.LoadVoiceStyle([]string{stylePath}, false)
	if err != nil {
		return nil, err
	}

	styleCache[voice] = style
	return style, nil
}

func closeStyles() {
	cacheMu.Lock()
	defer cacheMu.Unlock()
	for _, style := range styleCache {
		style.Destroy()
	}
}

func main() {
	// 1. Initialize Supertonic TTS engine
	params := supertonic.NewDefaultParameters()
	params.TotalStep = 32 // Consistent with previous tests or for balance
	params.Speed = 1.05
	params.SilenceDuration = 0.3

	engine, err := supertonic.NewTTS(params)
	if err != nil {
		log.Fatalf("Failed to initialize Supertonic TTS: %v", err)
	}
	defer engine.Close()
	defer closeStyles()

	// 2. Create MCP server
	s := server.NewMCPServer("supertonic-tts", "1.0.0")

	// 3. Define and Register Tool
	synthTool := mcp.NewTool("synthesize_speech",
		mcp.WithDescription("Convert input text to a speech audio wav file and save it to disk"),
		mcp.WithString("input_text",
			mcp.Description("text to synthesize speech from"),
			mcp.Required(),
		),
		mcp.WithString("output_filename",
			mcp.Description("name or path of the output WAV file (e.g., speech.wav)"),
			mcp.DefaultString("speech.wav"),
		),
		mcp.WithString("voice",
			mcp.Description("voice style name (e.g., F1, F5, M2)"),
			mcp.DefaultString("F1"),
		),
		mcp.WithString("lang",
			mcp.Description("language code (e.g., ko, en, es, pt, fr)"),
			mcp.DefaultString(""),
		),
		mcp.WithNumber("speed",
			mcp.Description("speed rate to synthesize speech (e.g., 1.0)"),
			mcp.DefaultNumber(1.0),
		),
	)

	s.AddTool(synthTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		inputText, err := request.RequireString("input_text")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Missing 'input_text': %v", err)), nil
		}

		outputFilename := request.GetString("output_filename", "speech.wav")
		voice := request.GetString("voice", "F1")
		lang := request.GetString("lang", "")
		speed := float32(request.GetFloat("speed", 1.0))

		// Create output file
		f, err := os.Create(outputFilename)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to create output file '%s': %v", outputFilename, err)), nil
		}
		defer f.Close()

		// Load or get cached style
		style, err := getStyle(voice, params.VoiceStyleDir)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to load voice style '%s': %v", voice, err)), nil
		}

		// Generate audio
		duration, err := engine.EncodeWavIOWithStyle(f, inputText, lang, speed, style)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to synthesize speech: %v", err)), nil
		}

		absPath, _ := filepath.Abs(outputFilename)
		result := struct {
			AudioSavedTo string  `json:"audio_saved_to"`
			Duration     float32 `json:"duration_seconds"`
		}{
			AudioSavedTo: absPath,
			Duration:     duration,
		}

		b, err := json.Marshal(result)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to marshal result: %v", err)), nil
		}

		return mcp.NewToolResultText(string(b)), nil
	})

	// 4. Start Server
	if err := server.ServeStdio(s); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}
