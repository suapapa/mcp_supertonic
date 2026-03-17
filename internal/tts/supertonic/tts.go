package supertonic

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

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

func (e *TTS) EncodeWavIOWithStyle(w io.WriteSeeker, text string, lang string, speed float32, style *Style) (float32, error) {
	if lang == "" {
		lang = guessLang(text)
	}

	wav, duration, err := e.textToSpeech.Call(text, lang, style, e.params.TotalStep, speed, e.params.SilenceDuration)
	if err != nil {
		return 0, fmt.Errorf("error generating speech: %w", err)
	}

	var wavOut []float64

	// For non-batch mode, wav is a single concatenated audio
	wavLen := int(float32(e.textToSpeech.SampleRate) * duration)
	wavOut = make([]float64, wavLen)
	for j := 0; j < wavLen && j < len(wav); j++ {
		wavOut[j] = float64(wav[j])
	}

	if err := writeWavFileIO(w, wavOut, e.textToSpeech.SampleRate); err != nil {
		return 0, fmt.Errorf("error writing wav file: %w", err)
	}

	return duration, nil
}

func (e *TTS) BatchEncodeWavIOWithStyle(w []io.WriteSeeker, text string, lang string, speed float32, style *Style) ([]float32, error) {
	bsz := len(w)
	if bsz == 0 {
		return nil, fmt.Errorf("no writers provided")
	}

	if lang == "" {
		lang = guessLang(text)
	}

	// Create batch inputs
	textList := make([]string, bsz)
	langList := make([]string, bsz)
	for i := 0; i < bsz; i++ {
		textList[i] = text
		langList[i] = lang
	}

	// Replicate style if needed
	var runStyle *Style
	var destroyStyle bool = false

	ttlShape := style.TtlTensor.GetShape()
	dpShape := style.DpTensor.GetShape()

	if ttlShape[0] == 1 && dpShape[0] == 1 && bsz > 1 {
		ttlData := style.TtlTensor.GetData()
		dpData := style.DpTensor.GetData()

		ttlSize := ttlShape[1] * ttlShape[2]
		dpSize := dpShape[1] * dpShape[2]

		newTtlData := make([]float32, int64(bsz)*ttlSize)
		newDpData := make([]float32, int64(bsz)*dpSize)

		for i := 0; i < bsz; i++ {
			copy(newTtlData[int64(i)*ttlSize:], ttlData)
			copy(newDpData[int64(i)*dpSize:], dpData)
		}

		newTtlShape := []int64{int64(bsz), ttlShape[1], ttlShape[2]}
		newDpShape := []int64{int64(bsz), dpShape[1], dpShape[2]}

		newTtlTensor, err := ort.NewTensor(newTtlShape, newTtlData)
		if err != nil {
			return nil, fmt.Errorf("failed to create replicated TTL tensor: %w", err)
		}

		newDpTensor, err := ort.NewTensor(newDpShape, newDpData)
		if err != nil {
			newTtlTensor.Destroy()
			return nil, fmt.Errorf("failed to create replicated DP tensor: %w", err)
		}

		runStyle = &Style{
			TtlTensor: newTtlTensor,
			DpTensor:  newDpTensor,
		}
		destroyStyle = true
	} else if int(ttlShape[0]) != bsz || int(dpShape[0]) != bsz {
		return nil, fmt.Errorf("style batch size mismatch: style contains %d items, but requested batch size is %d", ttlShape[0], bsz)
	} else {
		runStyle = style
	}

	if destroyStyle {
		defer runStyle.Destroy()
	}

	// Generate speech
	wav, duration, err := e.textToSpeech.Batch(textList, langList, runStyle, e.params.TotalStep, speed)
	if err != nil {
		return nil, fmt.Errorf("error generating batch speech: %w", err)
	}

	// Save outputs in parallel
	var wg sync.WaitGroup
	errCh := make(chan error, bsz)

	for i := 0; i < bsz; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			wavOut := extractWavSegment(wav, duration[idx], e.textToSpeech.SampleRate, idx, bsz)
			if err := writeWavFileIO(w[idx], wavOut, e.textToSpeech.SampleRate); err != nil {
				errCh <- fmt.Errorf("error writing wav to writer %d: %w", idx, err)
			}
		}(i)
	}

	wg.Wait()
	close(errCh)

	if len(errCh) > 0 {
		return duration, <-errCh
	}

	return duration, nil
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
		fname := fmt.Sprintf("%s.wav", SanitizeFilename(texts[i], 20))
		var wavOut []float64

		wavOut = extractWavSegment(wav, duration[i], e.textToSpeech.SampleRate, i, len(texts))

		outputPath := filepath.Join(saveDir, fname)
		if err := writeWavFile(outputPath, wavOut, e.textToSpeech.SampleRate); err != nil {
			return fmt.Errorf("error writing wav file: %w", err)
		}
	}

	return nil
}
