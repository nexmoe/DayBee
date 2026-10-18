#import <AppKit/AppKit.h>
#import <CoreAudio/CoreAudio.h>
#import <libproc.h>
#import <unistd.h>

static OSStatus readProperty(AudioObjectID object, AudioObjectPropertySelector selector, UInt32 size, void *value) {
    AudioObjectPropertyAddress address = {selector, kAudioObjectPropertyScopeGlobal, kAudioObjectPropertyElementMain};
    return AudioObjectGetPropertyData(object, &address, 0, NULL, &size, value);
}

// Audio clients may be browser helpers or XPC services inside an app bundle.
// Resolve the outer bundle so the timeline shows the application users know.
static NSBundle *ownerBundle(pid_t pid) {
    char path[PROC_PIDPATHINFO_MAXSIZE] = {0};
    if (proc_pidpath(pid, path, sizeof(path)) <= 0) return nil;
    NSString *executable = [NSString stringWithUTF8String:path];
    NSRange boundary = [executable rangeOfString:@".app/"];
    if (boundary.location == NSNotFound) return nil;
    return [NSBundle bundleWithPath:[executable substringToIndex:boundary.location + 4]];
}

static NSString *iconData(NSImage *image) {
    if (!image) return @"";
    NSBitmapImageRep *bitmap = [[NSBitmapImageRep alloc]
        initWithBitmapDataPlanes:NULL pixelsWide:32 pixelsHigh:32 bitsPerSample:8
        samplesPerPixel:4 hasAlpha:YES isPlanar:NO colorSpaceName:NSDeviceRGBColorSpace bytesPerRow:0 bitsPerPixel:0];
    [NSGraphicsContext saveGraphicsState];
    [NSGraphicsContext setCurrentContext:[NSGraphicsContext graphicsContextWithBitmapImageRep:bitmap]];
    [image drawInRect:NSMakeRect(0, 0, 32, 32) fromRect:NSZeroRect operation:NSCompositingOperationCopy fraction:1];
    [NSGraphicsContext restoreGraphicsState];
    NSData *png = [bitmap representationUsingType:NSBitmapImageFileTypePNG properties:@{}];
    [bitmap release];
    return png ? [@"data:image/png;base64," stringByAppendingString:[png base64EncodedStringWithOptions:0]] : @"";
}

char *daybeeMicrophoneApps(int *status) {
    @autoreleasepool {
        *status = 0;
        AudioObjectPropertyAddress address = {kAudioHardwarePropertyProcessObjectList,
            kAudioObjectPropertyScopeGlobal, kAudioObjectPropertyElementMain};
        if (!AudioObjectHasProperty(kAudioObjectSystemObject, &address)) {
            *status = -1;
            return NULL;
        }
        UInt32 size = 0;
        *status = AudioObjectGetPropertyDataSize(kAudioObjectSystemObject, &address, 0, NULL, &size);
        if (*status != noErr) return NULL;
        if (size == 0) return strdup("[]");
        AudioObjectID *objects = malloc(size);
        if (!objects) { *status = -2; return NULL; }
        *status = AudioObjectGetPropertyData(kAudioObjectSystemObject, &address, 0, NULL, &size, objects);
        if (*status != noErr) { free(objects); return NULL; }
        static NSCache *metadata;
        if (!metadata) { metadata = [[NSCache alloc] init]; metadata.countLimit = 256; }
        NSMutableArray *apps = [NSMutableArray array];
        NSBundle *selfBundle = ownerBundle(getpid());
        for (UInt32 i = 0; i < size / sizeof(AudioObjectID); i++) {
            UInt32 input = 0;
            pid_t pid = 0;
            OSStatus result = readProperty(objects[i], kAudioProcessPropertyIsRunningInput, sizeof(input), &input);
            if (result == kAudioHardwareBadObjectError) continue; // Client exited during the scan.
            if (result != noErr) { *status = result; break; }
            if (!input) continue;
            result = readProperty(objects[i], kAudioProcessPropertyPID, sizeof(pid), &pid);
            if (result == kAudioHardwareBadObjectError) continue;
            if (result != noErr) { *status = result; break; }
            if (pid <= 0 || pid == getpid()) continue;
            NSBundle *bundle = ownerBundle(pid);
            if (selfBundle && ([bundle.bundlePath isEqualToString:selfBundle.bundlePath] ||
                (selfBundle.bundleIdentifier.length && [bundle.bundleIdentifier isEqualToString:selfBundle.bundleIdentifier]))) continue;
            NSRunningApplication *running = [NSRunningApplication runningApplicationWithProcessIdentifier:pid];
            CFStringRef bundleID = NULL;
            readProperty(objects[i], kAudioProcessPropertyBundleID, sizeof(bundleID), &bundleID);
            NSString *identifier = bundle.bundleIdentifier ?: (NSString *)bundleID ?: running.bundleIdentifier;
            if (!identifier.length) identifier = running.executableURL.path;
            // A transient PID cannot identify an application across scans or launches.
            if (!identifier.length) { if (bundleID) CFRelease(bundleID); continue; }
            NSString *cacheKey = bundle.bundlePath ?: identifier;
            NSDictionary *info = [metadata objectForKey:cacheKey];
            if (!info) {
                NSString *name = [bundle objectForInfoDictionaryKey:@"CFBundleDisplayName"] ?: [bundle objectForInfoDictionaryKey:@"CFBundleName"] ?: running.localizedName ?: identifier;
                NSImage *image = bundle ? [[NSWorkspace sharedWorkspace] iconForFile:bundle.bundlePath] : running.icon;
                info = @{@"id": identifier, @"name": name, @"icon": iconData(image)};
                [metadata setObject:info forKey:cacheKey];
            }
            NSMutableDictionary *app = [[info mutableCopy] autorelease];
            app[@"pid"] = @(pid);
            [apps addObject:app];
            if (bundleID) CFRelease(bundleID);
        }
        free(objects);
        if (*status != noErr) return NULL;
        NSData *json = [NSJSONSerialization dataWithJSONObject:apps options:0 error:nil];
        return json ? strdup([[[NSString alloc] initWithData:json encoding:NSUTF8StringEncoding] autorelease].UTF8String) : NULL;
    }
}
