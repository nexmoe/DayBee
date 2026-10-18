package voiceprint

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	sherpa "github.com/k2-fsa/sherpa-onnx-go/sherpa_onnx"
)

func TestLoadCohortEmbedded(t *testing.T) {
	vectors, meta, err := LoadCohort()
	if err != nil {
		t.Fatal(err)
	}
	if len(vectors) != cohortVectorCount || meta.Vectors != cohortVectorCount || meta.Dim != embeddingDimension {
		t.Fatalf("count=%d dim=%d", len(vectors), meta.Dim)
	}
	if meta.Model != ModelName || meta.License != cohortLicense || meta.DatasetURL != cohortDatasetURL {
		t.Fatalf("provenance model=%s license=%s url=%s", meta.Model, meta.License, meta.DatasetURL)
	}
	if !sameStrings(meta.HeldOut, cohortHeldOutIDs) || !sameStrings(meta.Speakers, cohortSpeakerIDs) {
		t.Fatalf("speakers=%v held=%v", meta.Speakers, meta.HeldOut)
	}
	counts := map[string]int{}
	for i, vector := range vectors {
		if len(vector) != embeddingDimension {
			t.Fatalf("vector %d dim %d", i, len(vector))
		}
		if err := cohortNorm(vector); err != nil {
			t.Fatalf("vector %d: %v", i, err)
		}
		window := meta.Windows[i]
		counts[window.Speaker]++
		for _, held := range cohortHeldOutIDs {
			if window.Speaker == held || fileSpeaker(window.File) == held {
				t.Fatalf("held-out window %s", window.File)
			}
		}
	}
	for _, speaker := range cohortSpeakerIDs {
		if counts[speaker] != cohortPerSpeaker {
			t.Fatalf("%s has %d vectors", speaker, counts[speaker])
		}
	}
	vectors[0][0] = 0
	again, _, err := LoadCohort()
	if err != nil || again[0][0] == 0 {
		t.Fatal("caller mutated the cached cohort", err)
	}
}

func TestCohortRejectsInvalidInputs(t *testing.T) {
	meta := embeddedCohortMeta(t)
	blob := append([]byte(nil), cohortBlob...)
	if _, _, err := parseCohort(blob[:len(blob)-4], mustMeta(t, meta)); !errors.Is(err, errCohortSize) {
		t.Fatal("short blob", err)
	}
	meta.Model = "other.onnx"
	if _, _, err := parseCohort(blob, mustMeta(t, meta)); !errors.Is(err, errCohortModel) {
		t.Fatal("model", err)
	}
	meta = embeddedCohortMeta(t)
	meta.Windows[0].Speaker = "SSB0544"
	meta.Windows[0].File = "SSB0544_1.wav"
	if _, _, err := parseCohort(blob, mustMeta(t, meta)); !errors.Is(err, errCohortHeld) {
		t.Fatal("holdout", err)
	}
	meta = embeddedCohortMeta(t)
	meta.BlobSHA256 = "0000000000000000000000000000000000000000000000000000000000000000"
	if _, _, err := parseCohort(blob, mustMeta(t, meta)); !errors.Is(err, errCohortHash) {
		t.Fatal("hash", err)
	}
	nanBlob := append([]byte(nil), cohortBlob...)
	binary.LittleEndian.PutUint32(nanBlob[:4], math.Float32bits(float32(math.NaN())))
	nanMeta := embeddedCohortMeta(t)
	sum := sha256.Sum256(nanBlob)
	nanMeta.BlobSHA256 = hex.EncodeToString(sum[:])
	if _, _, err := parseCohort(nanBlob, mustMeta(t, nanMeta)); !errors.Is(err, errCohortFinite) {
		t.Fatal("nan", err)
	}
	zeroBlob := append([]byte(nil), cohortBlob...)
	for i := 0; i < embeddingDimension*4; i++ {
		zeroBlob[i] = 0
	}
	zeroMeta := embeddedCohortMeta(t)
	sum = sha256.Sum256(zeroBlob)
	zeroMeta.BlobSHA256 = hex.EncodeToString(sum[:])
	if _, _, err := parseCohort(zeroBlob, mustMeta(t, zeroMeta)); !errors.Is(err, errCohortNorm) {
		t.Fatal("zero", err)
	}
}

func TestCohortLicenseAndNotice(t *testing.T) {
	_, meta, err := LoadCohort()
	if err != nil {
		t.Fatal(err)
	}
	if meta.LicenseText != cohortLicenseText || meta.NoticeText != cohortNoticeText {
		t.Fatal("LoadCohort did not return the embedded license text")
	}
	licenseLines := strings.Count(meta.LicenseText, "\n")
	if licenseLines < 200 || licenseLines >= 500 {
		t.Fatalf("LICENSE.txt has %d lines", licenseLines)
	}
	for _, phrase := range []string{
		"Apache License",
		"Version 2.0, January 2004",
		"http://www.apache.org/licenses/",
		"APPENDIX: How to apply the Apache License to your work.",
	} {
		if !strings.Contains(meta.LicenseText, phrase) {
			t.Fatalf("LICENSE.txt missing %q", phrase)
		}
	}
	for _, phrase := range []string{
		"AISHELL-3",
		"https://openslr.org/93/",
		"Beijing Shell Shell Technology Co., Ltd.",
		"Apache License, Version 2.0",
		ModelName,
		"SSB0434", "SSB0544", "SSB0693", "SSB0700",
		"288",
	} {
		if !strings.Contains(meta.NoticeText, phrase) {
			t.Fatalf("NOTICE.txt missing %q", phrase)
		}
	}
}

func TestCohortWindowsMatchPublicWAV(t *testing.T) {
	dir := os.Getenv("DAYBEE_VOICEPRINT_COHORT_WAV")
	if dir == "" {
		t.Skip("set DAYBEE_VOICEPRINT_COHORT_WAV to compare bundled windows with AISHELL-3 wavs")
	}
	_, meta, err := LoadCohort()
	if err != nil {
		t.Fatal(err)
	}
	cache := map[string][]float32{}
	for _, window := range meta.Windows {
		path := filepath.Join(dir, window.File)
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(raw)
		if hex.EncodeToString(sum[:]) != window.FileSHA256 {
			t.Fatalf("%s file hash", window.File)
		}
		samples, ok := cache[window.File]
		if !ok {
			wave := sherpa.ReadWave(path)
			if wave == nil || wave.SampleRate != voiceSampleRate {
				t.Fatalf("unreadable %s", window.File)
			}
			samples = wave.Samples
			cache[window.File] = samples
		}
		if window.Offset < 0 || window.Offset+window.Samples > len(samples) {
			t.Fatalf("%s offset %d samples %d len %d", window.File, window.Offset, window.Samples, len(samples))
		}
		got, err := sha256PCM(samples[window.Offset : window.Offset+window.Samples])
		if err != nil || got != window.WindowSHA256 {
			t.Fatalf("%s@%d window hash %s != %s (%v)", window.File, window.Offset, got, window.WindowSHA256, err)
		}
	}
}

func embeddedCohortMeta(t *testing.T) CohortMeta {
	t.Helper()
	var meta CohortMeta
	if err := json.Unmarshal(cohortMetaJSON, &meta); err != nil {
		t.Fatal(err)
	}
	return meta
}

func mustMeta(t *testing.T, meta CohortMeta) []byte {
	t.Helper()
	raw, err := json.Marshal(meta)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func sha256PCM(samples []float32) (string, error) {
	buf := make([]byte, len(samples)*4)
	var le [4]byte
	for i, value := range samples {
		binary.LittleEndian.PutUint32(le[:], math.Float32bits(value))
		copy(buf[i*4:], le[:])
	}
	sum := sha256.Sum256(buf)
	return hex.EncodeToString(sum[:]), nil
}
