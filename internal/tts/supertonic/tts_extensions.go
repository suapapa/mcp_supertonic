package supertonic

import (
	"fmt"
	"io"
)

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

func guessLang(text string) string {
	for _, r := range text {
		// Hangul Syllables (AC00-D7AF), Jamo (1100-11FF), Compatibility Jamo (3130-318F)
		if (r >= 0xAC00 && r <= 0xD7AF) || (r >= 0x1100 && r <= 0x11FF) || (r >= 0x3130 && r <= 0x318F) {
			return "ko"
		}
	}

	// Smart guess for other AvailableLangs: es, pt, fr
	for _, r := range text {
		switch r {
		case 'ñ', '¿', '¡':
			return "es"
		case 'è', 'ê', 'ë', 'î', 'ï', 'ô', 'û', 'ù', 'œ', 'æ':
			return "fr"
		case 'ã', 'õ':
			return "pt"
		}
	}

	return "en"
}
