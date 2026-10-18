package desktop

import "github.com/egoist/mygo"

func (s *DesktopService) OpenRecorderWindow() {
	s.reveal()
}

func (s *DesktopService) windowForRole(role string) *mygo.Window {
	if role == "compact" {
		return s.window
	}
	return nil
}
