package desktop

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework AppKit
#import <AppKit/AppKit.h>
typedef struct {
    long number, collectionBehavior, level;
    int onActiveSpace, occluded, active, visibleWindowButtons;
} DayBeeWindowState;
static DayBeeWindowState daybeeWindowState(uintptr_t handle) {
    NSWindow *window = (NSWindow *)handle;
    int visibleWindowButtons = 0;
    for (NSNumber *type in @[@(NSWindowCloseButton), @(NSWindowMiniaturizeButton), @(NSWindowZoomButton)]) {
        NSButton *button = [window standardWindowButton:type.unsignedIntegerValue];
        if (button && !button.hiddenOrHasHiddenAncestor) visibleWindowButtons++;
    }
    return (DayBeeWindowState){
        window.windowNumber, window.collectionBehavior, window.level,
        window.onActiveSpace, !(window.occlusionState & NSWindowOcclusionStateVisible),
        NSApplication.sharedApplication.active, visibleWindowButtons
    };
}
*/
import "C"

import "github.com/egoist/mygo"

// Used only by the authenticated native acceptance endpoint.
func nativeWindowState(window *mygo.Window) map[string]any {
	var state C.DayBeeWindowState
	mygo.RunOnMain(func() { state = C.daybeeWindowState(C.uintptr_t(window.NativeHandle())) })
	return map[string]any{
		"number": int(state.number), "collectionBehavior": int(state.collectionBehavior), "level": int(state.level),
		"onActiveSpace": state.onActiveSpace != 0, "occluded": state.occluded != 0, "appActive": state.active != 0,
		"visibleWindowButtons": int(state.visibleWindowButtons),
	}
}
