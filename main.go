package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/suapapa/mcp_supertonic/internal/audioserve"
	"github.com/suapapa/mcp_supertonic/internal/tts/supertonic"
)

const (
	mcpSupertonicVersion = "1.2.0"
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
	port := flag.Int("port", 0, "Port to start SSE server on. 0 means Stdio.")
	resourceTTL := flag.Duration("resourceTTL", time.Hour, "TTL for tts://output/{id} resources/read mappings")

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

	audioReg := audioserve.NewRegistry(*resourceTTL)

	// 2. Create MCP server (resources: expose WAV via resources/read for SSE clients)
	s := server.NewMCPServer("supertonic-tts", mcpSupertonicVersion, server.WithResourceCapabilities(false, true))

	s.AddResourceTemplate(
		mcp.NewResourceTemplate(audioserve.TemplateString, "tts-output-wav",
			mcp.WithTemplateDescription("Temporary synthesized WAV; fetch bytes via MCP resources/read until TTL expires."),
			mcp.WithTemplateMIMEType("audio/wav"),
		),
		func(ctx context.Context, req mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
			blob, mime, err := audioReg.ReadBlob(req.Params.URI)
			if err != nil {
				return nil, err
			}
			return []mcp.ResourceContents{
				mcp.BlobResourceContents{
					URI:      req.Params.URI,
					MIMEType: mime,
					Blob:     blob,
				},
			}, nil
		},
	)

	// 3. Define and Register Tool
	synthTool := mcp.NewTool("synthesize_speech",
		mcp.WithDescription("Convert input text to a speech audio wav file and save it on the server. "+
			"Over SSE, local paths are not visible to clients — use resource_uri with MCP resources/read or embed_audio=true for audio_base64 in the JSON text."),
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
		mcp.WithBoolean("embed_audio",
			mcp.Description("if true, include the WAV bytes as audio_base64 inside the JSON text result (Gemini/Vertex function responses do not accept audio/wav parts; use this or resource_uri)"),
			mcp.DefaultBool(false),
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
		embedAudio := request.GetBool("embed_audio", false)

		f, err := os.Create(outputFilename)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to create output file '%s': %v", outputFilename, err)), nil
		}

		style, err := styleManager.Get(voice)
		if err != nil {
			f.Close()
			return mcp.NewToolResultError(fmt.Sprintf("Failed to load voice style '%s': %v", voice, err)), nil
		}

		duration, err := engine.EncodeWavIOWithStyle(f, inputText, lang, speed, style)
		if err != nil {
			f.Close()
			return mcp.NewToolResultError(fmt.Sprintf("Failed to synthesize speech: %v", err)), nil
		}
		if err := f.Close(); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to close output file: %v", err)), nil
		}

		absPath, _ := filepath.Abs(outputFilename)
		resourceURI := audioReg.Register(absPath)

		result := struct {
			AudioSavedTo string  `json:"audio_saved_to"`
			Duration     float32 `json:"duration_seconds"`
			ResourceURI  string  `json:"resource_uri"`
			AudioBase64  string  `json:"audio_base64,omitempty"`
			MimeType     string  `json:"mime_type,omitempty"`
		}{
			AudioSavedTo: absPath,
			Duration:     duration,
			ResourceURI:  resourceURI,
		}

		if embedAudio {
			raw, err := os.ReadFile(absPath)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to read output for embedding: %v", err)), nil
			}
			result.AudioBase64 = base64.StdEncoding.EncodeToString(raw)
			result.MimeType = "audio/wav"
		}

		b, err := json.Marshal(result)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to marshal result: %v", err)), nil
		}

		return mcp.NewToolResultText(string(b)), nil
	})

	batchSynthTool := mcp.NewTool("batch_synthesize_speech",
		mcp.WithDescription("Convert input text to multiple speech audio wav files in a batch with variations to support selection. "+
			"Each saved_files entry includes resource_uri for MCP resources/read (SSE-friendly). embed_audio embeds WAV only when batch_cnt is 1."),
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
		mcp.WithString("output_dir",
			mcp.Description("directory path to save output WAV files (e.g., outputs)"),
			mcp.DefaultString("."),
		),
		mcp.WithBoolean("embed_audio",
			mcp.Description("if true and batch_cnt is 1, include WAV as audio_base64 in the JSON text result; for batch_cnt>1 use resource_uri for each file"),
			mcp.DefaultBool(false),
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
		outputDir := request.GetString("output_dir", ".")
		embedAudio := request.GetBool("embed_audio", false)

		// Create output directory if it doesn't exist
		if err := os.MkdirAll(outputDir, 0755); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to create output directory '%s': %v", outputDir, err)), nil
		}

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
			fpath := filepath.Join(outputDir, fname)
			f, err := os.Create(fpath)
			if err != nil {
				// Clean up previous files
				for j := 0; j < i; j++ {
					files[j].Close()
				}
				return mcp.NewToolResultError(fmt.Sprintf("Failed to create file '%s': %v", fpath, err)), nil
			}
			writers[i] = f
			filenames[i] = fpath
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
			Path        string  `json:"path"`
			Duration    float32 `json:"duration_seconds"`
			ResourceURI string  `json:"resource_uri"`
		}
		var savedFiles []SavedFile

		for i := 0; i < batchCnt; i++ {
			absPath, _ := filepath.Abs(filenames[i])
			savedFiles = append(savedFiles, SavedFile{
				Path:        absPath,
				Duration:    durations[i],
				ResourceURI: audioReg.Register(absPath),
			})
		}

		payload := struct {
			SavedFiles  []SavedFile `json:"saved_files"`
			EmbedNote   string      `json:"embed_note,omitempty"`
			AudioBase64 string      `json:"audio_base64,omitempty"`
			MimeType    string      `json:"mime_type,omitempty"`
		}{SavedFiles: savedFiles}

		if embedAudio && batchCnt > 1 {
			payload.EmbedNote = "embed_audio only applies when batch_cnt is 1; use resource_uri with resources/read for each output."
		}

		if embedAudio && batchCnt == 1 {
			abs0, _ := filepath.Abs(filenames[0])
			raw, err := os.ReadFile(abs0)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to read output for embedding: %v", err)), nil
			}
			payload.AudioBase64 = base64.StdEncoding.EncodeToString(raw)
			payload.MimeType = "audio/wav"
		}

		b, err := json.Marshal(payload)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to marshal result: %v", err)), nil
		}

		return mcp.NewToolResultText(string(b)), nil
	})

	// 4. Start Server
	if *port > 0 {
		// Periodic JSON-RPC pings on the SSE stream prevent idle timeouts (proxies, LB, some clients).
		sseServer := server.NewSSEServer(s, server.WithKeepAliveInterval(25*time.Second))
		addr := fmt.Sprintf(":%d", *port)
		log.Printf("Starting SSE server on %s", addr)
		if err := sseServer.Start(addr); err != nil {
			log.Fatalf("Server error: %v", err)
		}
	} else {
		if err := server.ServeStdio(s); err != nil {
			log.Fatalf("Server error: %v", err)
		}
	}
}
