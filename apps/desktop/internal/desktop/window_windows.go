package desktop

import "github.com/egoist/mygo"

func nativeWindowState(_ *mygo.Window) map[string]any { return nil }

func installWindowCompatibility(window *mygo.Window, _ int) {
	// mygo 0.3.4 leaves invisible resize borders inside frameless bounds.
	// Match the requested page size using measured DIPs, never fixed pixels.
	width, height := window.Size()
	contentWidth, contentHeight := window.ContentSize()
	window.SetSize(width+max(0, width-contentWidth), height+max(0, height-contentHeight))
}
