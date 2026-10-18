package desktop

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework AppKit
#import <AppKit/AppKit.h>
#include <stdint.h>

@interface DayBeeLegacyDragView : NSView
@end

@implementation DayBeeLegacyDragView
- (BOOL)isOpaque { return NO; }
- (BOOL)acceptsFirstMouse:(NSEvent *)event { return YES; }
- (NSView *)hitTest:(NSPoint)point {
    NSPoint local = [self convertPoint:point fromView:self.superview];
    if (!NSPointInRect(local, self.bounds)) return nil;
    for (NSNumber *type in @[@(NSWindowCloseButton), @(NSWindowMiniaturizeButton), @(NSWindowZoomButton)]) {
        NSButton *button = [self.window standardWindowButton:type.unsignedIntegerValue];
        if (button && !button.hidden && NSPointInRect(local, [self convertRect:button.bounds fromView:button])) return nil;
    }
    return self;
}
- (void)mouseDown:(NSEvent *)event {
    // Use the current event: WKContentView can consume mouseDown before
    // mygo's WKWebView subclass records the event for a CSS drag request.
    if (event.clickCount == 2) {
        NSString *action = [[NSUserDefaults standardUserDefaults] stringForKey:@"AppleActionOnDoubleClick"];
        if ([action isEqualToString:@"Minimize"]) [self.window performMiniaturize:nil];
        else if (![action isEqualToString:@"None"]) [self.window performZoom:nil];
    } else {
        [self.window performWindowDragWithEvent:event];
    }
}
@end

static uintptr_t daybeeInstallWindowDrag(uintptr_t handle, int height) {
    NSView *content = ((NSWindow *)handle).contentView;
    NSRect bounds = content.bounds;
    // Match each window's CSS drag region without covering its controls.
    NSRect frame = NSMakeRect(NSMinX(bounds), NSMaxY(bounds) - height, NSWidth(bounds), height);
    DayBeeLegacyDragView *view = [[DayBeeLegacyDragView alloc] initWithFrame:frame];
    view.autoresizingMask = NSViewWidthSizable | NSViewMinYMargin;
    [content addSubview:view positioned:NSWindowAbove relativeTo:nil];
    return (uintptr_t)view;
}

static void daybeeRemoveWindowDrag(uintptr_t handle) {
    NSView *view = (NSView *)handle;
    [view removeFromSuperview];
    [view release];
}
*/
import "C"

import (
	"log/slog"

	"github.com/egoist/mygo"
)

func installWindowCompatibility(window *mygo.Window, height int) {
	var view C.uintptr_t
	mygo.RunOnMain(func() { view = C.daybeeInstallWindowDrag(C.uintptr_t(window.NativeHandle()), C.int(height)) })
	if view == 0 {
		slog.Error("Unable to install the native title-bar drag compatibility handler")
		return
	}
	window.OnClosed(func() { mygo.RunOnMain(func() { C.daybeeRemoveWindowDrag(view) }) })
}
