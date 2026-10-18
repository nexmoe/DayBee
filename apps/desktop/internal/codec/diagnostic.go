package codec

import (
	"context"
	"encoding/json"
	"errors"
	"io"
)

// Diagnostic is used by the package verifier to exercise the codec compiled
// into the shipped executable, with an empty PATH and isolated temporary files.
func Diagnostic(args []string, output io.Writer) error {
	ctx := context.Background()
	var source string
	switch {
	case len(args) == 3 && args[0] == "encode":
		if err := EncodeFLAC(ctx, args[1], args[2]); err != nil {
			return err
		}
		source = args[2]
	case len(args) == 2 && args[0] == "decode":
		source = args[1]
	default:
		return errors.New("audio diagnostic expects encode <wav> <flac> or decode <recording>")
	}
	samples, rate, err := ReadMono(ctx, source)
	if err != nil {
		return err
	}
	return json.NewEncoder(output).Encode(struct {
		SampleRate int       `json:"sampleRate"`
		Samples    []float32 `json:"samples"`
	}{rate, samples})
}
