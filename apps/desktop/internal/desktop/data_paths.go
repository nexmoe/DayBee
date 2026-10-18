package desktop

import (
	"os"
	"path/filepath"

	"github.com/egoist/mygo"
)

type dataPaths struct {
	root      string
	documents string
	legacy    []string
}

// Development and packaged apps share data unless isolation is requested.
func (s *DesktopService) dataPaths(appData, documents string) dataPaths {
	root := os.Getenv("DAYBEE_STORE_DIR")
	if root == "" {
		root = filepath.Join(appData, "DayBee")
		if s.test {
			root = filepath.Join(appData, "DayBee Development")
		}
	}
	paths := dataPaths{root: root, documents: documents, legacy: []string{}}
	if s.isolated() {
		paths.documents = root
	} else {
		paths.legacy = []string{
			filepath.Join(appData, "Eve Recorder"),
			filepath.Join(appData, "RecBee"),
			filepath.Join(appData, "eve-desktop"),
		}
	}
	return paths
}

func (s *DesktopService) isolated() bool {
	return os.Getenv("DAYBEE_STORE_DIR") != "" || s.test
}

func (s *DesktopService) shouldApplyLoginItem() bool {
	return !mygo.IsDev() && !s.isolated()
}

func (s *DesktopService) recordingRoots(output string) []string {
	return append([]string{output, s.defaultOutput}, s.legacyOutputs...)
}
