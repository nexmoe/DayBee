package voiceprint

import (
	"crypto/sha256"
	_ "embed"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sync"
)

//go:embed cohort_eres2netv2.f32
var cohortBlob []byte

//go:embed cohort_meta.json
var cohortMetaJSON []byte

//go:embed LICENSE.txt
var cohortLicenseText string

//go:embed NOTICE.txt
var cohortNoticeText string

const (
	cohortVectorCount = 288
	cohortPerSpeaker  = 32
	cohortFormat      = "float32-le"
	cohortDatasetURL  = "https://openslr.org/93/"
	cohortLicense     = "Apache-2.0"
)

var cohortSpeakerIDs = []string{
	"SSB0193", "SSB0407", "SSB0686", "SSB0702", "SSB0711",
	"SSB0716", "SSB0717", "SSB0736", "SSB0749",
}

var cohortHeldOutIDs = []string{"SSB0434", "SSB0544", "SSB0693", "SSB0700"}

var (
	errCohortModel  = errors.New("voiceprint cohort model does not match")
	errCohortSize   = errors.New("voiceprint cohort size is invalid")
	errCohortFinite = errors.New("voiceprint cohort vector is non-finite")
	errCohortNorm   = errors.New("voiceprint cohort vector norm is invalid")
	errCohortHeld   = errors.New("voiceprint cohort contains a held-out speaker")
	errCohortHash   = errors.New("voiceprint cohort hash does not match")
)

// CohortWindow is one public-speech window that produced a bundled vector.
type CohortWindow struct {
	Speaker      string `json:"speaker"`
	File         string `json:"file"`
	Offset       int    `json:"offset"`
	Samples      int    `json:"samples"`
	FileSHA256   string `json:"fileSHA256"`
	WindowSHA256 string `json:"windowSHA256"`
}

// CohortMeta is the provenance record embedded with the cohort.
type CohortMeta struct {
	Format            string         `json:"format"`
	Model             string         `json:"model"`
	ModelSHA256       string         `json:"modelSHA256"`
	Dataset           string         `json:"dataset"`
	DatasetURL        string         `json:"datasetURL"`
	License           string         `json:"license"`
	Dim               int            `json:"dim"`
	Speakers          []string       `json:"speakers"`
	VectorsPerSpeaker int            `json:"vectorsPerSpeaker"`
	Vectors           int            `json:"vectors"`
	HeldOut           []string       `json:"heldOut"`
	SampleRate        int            `json:"sampleRate"`
	WindowSeconds     float64        `json:"windowSeconds"`
	MinWindowSeconds  float64        `json:"minWindowSeconds"`
	HopSeconds        float64        `json:"hopSeconds"`
	Selection         string         `json:"selection"`
	WindowRule        string         `json:"windowRule"`
	BlobSHA256        string         `json:"blobSHA256"`
	Windows           []CohortWindow `json:"windows"`
	LicenseText       string         `json:"-"`
	NoticeText        string         `json:"-"`
}

var (
	cohortOnce    sync.Once
	cohortVectors [][]float32
	cohortMeta    CohortMeta
	cohortErr     error
)

// LoadCohort returns the bundled 288×192 impostor cohort and its provenance.
// The slices are copies; later callers cannot change the cached cohort.
func LoadCohort() ([][]float32, CohortMeta, error) {
	cohortOnce.Do(func() {
		cohortVectors, cohortMeta, cohortErr = parseCohort(cohortBlob, cohortMetaJSON)
	})
	if cohortErr != nil {
		return nil, CohortMeta{}, cohortErr
	}
	return copyCohortVectors(cohortVectors), copyCohortMeta(cohortMeta), nil
}

func parseCohort(blob, raw []byte) ([][]float32, CohortMeta, error) {
	var meta CohortMeta
	if err := json.Unmarshal(raw, &meta); err != nil {
		return nil, CohortMeta{}, err
	}
	vectors, err := validateCohort(blob, meta)
	if err != nil {
		return nil, CohortMeta{}, err
	}
	meta.LicenseText = cohortLicenseText
	meta.NoticeText = cohortNoticeText
	return vectors, meta, nil
}

func validateCohort(blob []byte, meta CohortMeta) ([][]float32, error) {
	if meta.Model != ModelName || meta.Format != cohortFormat || meta.License != cohortLicense || meta.DatasetURL != cohortDatasetURL {
		return nil, errCohortModel
	}
	if meta.ModelSHA256 == "" || !hex64(meta.ModelSHA256) || meta.Dataset != "AISHELL-3" {
		return nil, errCohortModel
	}
	if meta.Dim != embeddingDimension || meta.Vectors != cohortVectorCount || meta.VectorsPerSpeaker != cohortPerSpeaker || meta.SampleRate != voiceSampleRate {
		return nil, errCohortSize
	}
	if meta.WindowSeconds != 1.5 || meta.MinWindowSeconds != 0.8 || meta.HopSeconds != 0.5 {
		return nil, errCohortSize
	}
	if !sameStrings(meta.Speakers, cohortSpeakerIDs) || !sameStrings(meta.HeldOut, cohortHeldOutIDs) {
		return nil, errCohortSize
	}
	if err := validateCohortWindows(meta.Windows); err != nil {
		return nil, err
	}
	if len(blob) != cohortVectorCount*embeddingDimension*4 {
		return nil, errCohortSize
	}
	sum := sha256.Sum256(blob)
	if hex.EncodeToString(sum[:]) != meta.BlobSHA256 {
		return nil, errCohortHash
	}
	return decodeCohort(blob)
}

func validateCohortWindows(windows []CohortWindow) error {
	if len(windows) != cohortVectorCount {
		return errCohortSize
	}
	seen := map[string]bool{}
	for i, speaker := range cohortSpeakerIDs {
		for j := 0; j < cohortPerSpeaker; j++ {
			window := windows[i*cohortPerSpeaker+j]
			if heldSpeaker(window.Speaker) || heldSpeaker(fileSpeaker(window.File)) {
				return errCohortHeld
			}
			if window.Speaker != speaker {
				return errCohortSize
			}
			if window.Offset < 0 || window.Samples < minWindowSamples || window.Samples > windowSamples {
				return errCohortSize
			}
			if !hex64(window.FileSHA256) || !hex64(window.WindowSHA256) || window.File == "" {
				return errCohortHash
			}
			key := window.File + "#" + fmt.Sprint(window.Offset)
			if seen[key] {
				return errCohortSize
			}
			seen[key] = true
		}
	}
	return nil
}

func decodeCohort(blob []byte) ([][]float32, error) {
	vectors := make([][]float32, cohortVectorCount)
	for i := range vectors {
		raw := blob[i*embeddingDimension*4 : (i+1)*embeddingDimension*4]
		vector := make([]float32, embeddingDimension)
		for j := range vector {
			vector[j] = math.Float32frombits(binary.LittleEndian.Uint32(raw[j*4 : (j+1)*4]))
		}
		if err := cohortNorm(vector); err != nil {
			return nil, err
		}
		vectors[i] = vector
	}
	return vectors, nil
}

func cohortNorm(vector []float32) error {
	if len(vector) != embeddingDimension {
		return errCohortSize
	}
	var sum float64
	for _, value := range vector {
		if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
			return errCohortFinite
		}
		sum += float64(value) * float64(value)
	}
	if math.IsNaN(sum) || math.IsInf(sum, 0) || !(sum > 0) {
		return errCohortNorm
	}
	return nil
}

func copyCohortVectors(src [][]float32) [][]float32 {
	out := make([][]float32, len(src))
	for i, vector := range src {
		out[i] = append([]float32(nil), vector...)
	}
	return out
}

func copyCohortMeta(meta CohortMeta) CohortMeta {
	meta.Speakers = append([]string(nil), meta.Speakers...)
	meta.HeldOut = append([]string(nil), meta.HeldOut...)
	meta.Windows = append([]CohortWindow(nil), meta.Windows...)
	return meta
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func heldSpeaker(id string) bool {
	for _, speaker := range cohortHeldOutIDs {
		if id == speaker {
			return true
		}
	}
	return false
}

func fileSpeaker(name string) string {
	for i := len(name) - 1; i >= 0; i-- {
		if name[i] == '_' {
			return name[:i]
		}
	}
	return name
}

func hex64(value string) bool {
	if len(value) != 64 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}
