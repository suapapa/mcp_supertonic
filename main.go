package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/suapapa/mcp_supertonic/internal/tts/supertonic"
)

type StyleCache struct {
	cache    map[string]*supertonic.Style
	mu       sync.Mutex
	voiceDir string
}

func NewStyleCache(voiceDir string) *StyleCache {
	return &StyleCache{
		cache:    make(map[string]*supertonic.Style),
		voiceDir: voiceDir,
	}
}

func (c *StyleCache) Get(voice string) (*supertonic.Style, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if style, ok := c.cache[voice]; ok {
		return style, nil
	}

	voiceFile := voice
	if !strings.HasSuffix(voiceFile, ".json") {
		voiceFile = voiceFile + ".json"
	}
	stylePath := filepath.Join(c.voiceDir, voiceFile)

	if _, err := os.Stat(stylePath); err != nil {
		return nil, fmt.Errorf("voice style file not found: %s", stylePath)
	}

	style, err := supertonic.LoadVoiceStyle([]string{stylePath}, false)
	if err != nil {
		return nil, err
	}

	c.cache[voice] = style
	return style, nil
}

func (c *StyleCache) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, style := range c.cache {
		style.Destroy()
	}
}

func main() {
	// Flags for default values
	defTotalStep := flag.Int("defaultTotalStep", 24, "Default total step (5~32)")
	defSpeed := flag.Float64("defaultSpeed", 1.3, "Default speed rate")
	defSilenceDuration := flag.Float64("defaultSilenceDuration", 0.3, "Default silence duration")
	defVoice := flag.String("defaultVoice", "F1", "Default voice style (e.g., F1, F5, M2)")

	flag.Parse()

	// 1. Initialize Supertonic TTS engine
	params := supertonic.NewDefaultParameters()
	params.TotalStep = *defTotalStep
	params.Speed = float32(*defSpeed)
	params.SilenceDuration = float32(*defSilenceDuration)

	engine, err := supertonic.NewTTS(params)
	if err != nil {
		log.Fatalf("Failed to initialize Supertonic TTS: %v", err)
	}
	defer engine.Close()

	styleManager := NewStyleCache(params.VoiceStyleDir)
	defer styleManager.Close()

	// 2. Create MCP server
	s := server.NewMCPServer("supertonic-tts", "1.1.0")

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
			mcp.DefaultString(*defVoice),
		),
		mcp.WithString("lang",
			mcp.Description("language code (e.g., ko, en, es, pt, fr)"),
			mcp.DefaultString(""),
		),
		mcp.WithNumber("speed",
			mcp.Description("speed rate to synthesize speech (e.g., 1.3)"),
			mcp.DefaultNumber(*defSpeed),
		),
	)

	s.AddTool(synthTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		inputText, err := request.RequireString("input_text")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Missing 'input_text': %v", err)), nil
		}

		outputFilename := request.GetString("output_filename", "speech.wav")
		voice := request.GetString("voice", *defVoice)
		lang := request.GetString("lang", "")
		speed := float32(request.GetFloat("speed", *defSpeed))

		// Create output file
		f, err := os.Create(outputFilename)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to create output file '%s': %v", outputFilename, err)), nil
		}
		defer f.Close()

		// Load or get cached style
		style, err := styleManager.Get(voice)
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

	batchSynthTool := mcp.NewTool("batch_synthesize_speech",
		mcp.WithDescription("Convert input text to multiple speech audio wav files in a batch with variations to support selection"),
		mcp.WithString("input_text",
			mcp.Description("text to synthesize speech from"),
			mcp.Required(),
		),
		mcp.WithNumber("batch_cnt",
			mcp.Description("number of variations to generate (e.g., 3)"),
			mcp.DefaultNumber(3),
		),
		mcp.WithString("voice",
			mcp.Description("voice style name (e.g., F1, F5, M2)"),
			mcp.DefaultString(*defVoice),
		),
		mcp.WithString("lang",
			mcp.Description("language code (e.g., ko, en, es, pt, fr)"),
			mcp.DefaultString(""),
		),
		mcp.WithNumber("speed",
			mcp.Description("speed rate to synthesize speech (e.g., 1.3)"),
			mcp.DefaultNumber(*defSpeed),
		),
	)

	s.AddTool(batchSynthTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		inputText, err := request.RequireString("input_text")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Missing 'input_text': %v", err)), nil
		}

		batchCnt := int(request.GetFloat("batch_cnt", 3))
		if batchCnt < 1 {
			batchCnt = 1
		}

		voice := request.GetString("voice", *defVoice)
		lang := request.GetString("lang", "")
		speed := float32(request.GetFloat("speed", *defSpeed))

		// Load or get cached style
		style, err := styleManager.Get(voice)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to load voice style '%s': %v", voice, err)), nil
		}

		writers := make([]io.WriteSeeker, batchCnt)
		filenames := make([]string, batchCnt)
		files := make([]*os.File, batchCnt)

		baseName := supertonic.SanitizeFilename(inputText, 20)
		if baseName == "" {
			baseName = "speech"
		}

		for i := 0; i < batchCnt; i++ {
			fname := fmt.Sprintf("%s_%d.wav", baseName, i+1)
			f, err := os.Create(fname)
			if err != nil {
				// Clean up previous files
				for j := 0; j < i; j++ {
					files[j].Close()
				}
				return mcp.NewToolResultError(fmt.Sprintf("Failed to create file '%s': %v", fname, err)), nil
			}
			writers[i] = f
			filenames[i] = fname
			files[i] = f
		}

		// Generate audio (Batch mode)
		durations, err := engine.BatchEncodeWavIOWithStyle(writers, inputText, lang, speed, style)

		// Always close files
		for _, f := range files {
			f.Close()
		}

		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to synthesize speech batch: %v", err)), nil
		}

		type SavedFile struct {
			Path     string  `json:"path"`
			Duration float32 `json:"duration_seconds"`
		}
		var savedFiles []SavedFile

		for i := 0; i < batchCnt; i++ {
			absPath, _ := filepath.Abs(filenames[i])
			savedFiles = append(savedFiles, SavedFile{
				Path:     absPath,
				Duration: durations[i],
			})
		}

		b, err := json.Marshal(struct {
			SavedFiles []SavedFile `json:"saved_files"`
		}{SavedFiles: savedFiles})
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
