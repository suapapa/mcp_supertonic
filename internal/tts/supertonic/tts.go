package supertonic

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	ort "github.com/yalue/onnxruntime_go"
)

// Parameters holds command line arguments
type Parameters struct {
	UseGPU          bool
	ONNXDir         string
	VoiceStyleDir   string
	TotalStep       int
	Speed           float32
	SilenceDuration float32
	VoiceStyles     []string
	Langs           []string
}

func NewDefaultParameters() *Parameters {
	home, _ := os.UserHomeDir()
	baseDir := filepath.Join(home, ".local", "share", "supertonic2")

	onnxDir := filepath.Join(baseDir, "onnx")
	voiceStyleDir := filepath.Join(baseDir, "voice_styles")
	voiceStyle := "F1.json"

	// Fallback to relative path if absolute path does not exist
	if _, err := os.Stat(onnxDir); err != nil {
		onnxDir = "assets/supertonic2/onnx"
	}
	// if _, err := os.Stat(voiceStyle); err != nil {
	// 	voiceStyle = "assets/supertonic2/voice_styles/F5.json"
	// }

	cfgs := &Parameters{
		UseGPU:          false,
		ONNXDir:         onnxDir,
		VoiceStyleDir:   voiceStyleDir,
		TotalStep:       24, // 5~32 // Higher values improve pronunciation accuracy and stability, but take longer.
		Speed:           1.05,
		SilenceDuration: 0.3,
		VoiceStyles:     []string{voiceStyle},
		Langs:           []string{"ko"},
	}

	return cfgs
}

type TTS struct {
	params       *Parameters
	onnxCfgs     *Config
	textToSpeech *tts
	voiceStyle   *Style
}

func NewTTS(params *Parameters) (*TTS, error) {
	// Initialize ONNX Runtime
	if err := InitializeONNXRuntime(); err != nil {
		return nil, fmt.Errorf("error initializing ONNX Runtime: %v", err)
	}

	// Load config
	cfg, err := LoadCfgs(params.ONNXDir)
	if err != nil {
		return nil, fmt.Errorf("error loading config: %v", err)
	}

	// Load TTS components
	textToSpeech, err := LoadTextToSpeech(params.ONNXDir, params.UseGPU, cfg)
	if err != nil {
		return nil, fmt.Errorf("error loading TTS components: %v", err)
	}

	loadVoiceStyle := func() []string {
		ret := make([]string, len(params.VoiceStyles))
		for i, v := range params.VoiceStyles {
			if !strings.HasSuffix(v, ".json") {
				v = v + ".json"
			}
			ret[i] = filepath.Join(params.VoiceStyleDir, v)
		}
		return ret
	}

	style, err := LoadVoiceStyle(loadVoiceStyle(), false)
	if err != nil {
		return nil, fmt.Errorf("error loading voice styles: %v", err)
	}

	return &TTS{
		params:       params,
		onnxCfgs:     &cfg,
		textToSpeech: textToSpeech,
		voiceStyle:   style,
	}, nil
}

func (e *TTS) Close() {
	if e.voiceStyle != nil {
		e.voiceStyle.Destroy()
	}
	if e.textToSpeech != nil {
		e.textToSpeech.Destroy()
	}
	ort.DestroyEnvironment()
}

func (e *TTS) EncodeWavIO(w io.WriteSeeker, text string) error {
	_, err := e.EncodeWavIOWithStyle(w, text, e.params.Langs[0], e.params.Speed, e.voiceStyle)
	return err
}

func (e *TTS) BatchEncodeToFiles(saveDir string, texts []string) error {
	// --- 5. Synthesize speech --- //
	if err := os.MkdirAll(saveDir, 0755); err != nil {
		return fmt.Errorf("error creating save directory: %w", err)
	}

	var wav []float32
	var duration []float32

	w, d, err := e.textToSpeech.Batch(texts, e.params.Langs, e.voiceStyle, e.params.TotalStep, e.params.Speed)
	if err != nil {
		return fmt.Errorf("error generating batch speech: %w", err)
	}
	wav = w
	duration = d

	// Save outputs
	for i := 0; i < len(texts); i++ {
		fname := fmt.Sprintf("%s.wav", sanitizeFilename(texts[i], 20))
		var wavOut []float64

		wavOut = extractWavSegment(wav, duration[i], e.textToSpeech.SampleRate, i, len(texts))

		outputPath := filepath.Join(saveDir, fname)
		if err := writeWavFile(outputPath, wavOut, e.textToSpeech.SampleRate); err != nil {
			return fmt.Errorf("error writing wav file: %w", err)
		}
	}

	return nil
}
