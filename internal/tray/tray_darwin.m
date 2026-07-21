//go:build darwin

#import <AppKit/AppKit.h>
#include <stdio.h>
#include "tray_darwin.h"

extern void couchpilotTrayCommand(int command);

static NSStatusItem *cp_status_item = nil;
static id cp_tray_delegate = nil;

static void cp_tray_cleanup(void) {
    if (cp_status_item != nil) {
        [NSStatusBar.systemStatusBar removeStatusItem:cp_status_item];
        [cp_status_item release];
        cp_status_item = nil;
    }
    [cp_tray_delegate release];
    cp_tray_delegate = nil;
}

static void cp_log_tray_state(const char *phase) {
    NSApplication *application = NSApplication.sharedApplication;
    NSStatusBarButton *button = cp_status_item.button;
    NSWindow *window = button.window;
    NSString *bundle_identifier = NSBundle.mainBundle.bundleIdentifier ?: @"(none)";
    NSString *title = button.title ?: @"";
    fprintf(stderr,
        "tray[%s]: main_thread=%d app_running=%d app_active=%d policy=%ld "
        "bundle=%s status_item=%d length=%.1f button=%d hidden=%d alpha=%.2f "
        "image=%d title=%s frame={%.1f,%.1f,%.1f,%.1f} window=%d "
        "window_visible=%d window_number=%ld screen=%d menu=%d\n",
        phase,
        [NSThread isMainThread] ? 1 : 0,
        application.isRunning ? 1 : 0,
        application.isActive ? 1 : 0,
        (long)application.activationPolicy,
        bundle_identifier.UTF8String,
        cp_status_item != nil ? 1 : 0,
        cp_status_item != nil ? cp_status_item.length : -1.0,
        button != nil ? 1 : 0,
        button != nil && button.hidden ? 1 : 0,
        button != nil ? button.alphaValue : -1.0,
        button.image != nil ? 1 : 0,
        title.UTF8String,
        button != nil ? button.frame.origin.x : -1.0,
        button != nil ? button.frame.origin.y : -1.0,
        button != nil ? button.frame.size.width : -1.0,
        button != nil ? button.frame.size.height : -1.0,
        window != nil ? 1 : 0,
        window != nil && window.visible ? 1 : 0,
        window != nil ? (long)window.windowNumber : -1L,
        window.screen != nil ? 1 : 0,
        cp_status_item.menu != nil ? 1 : 0);
    fflush(stderr);
}

@interface CPTrayDelegate : NSObject
- (void)handleMenuItem:(NSMenuItem *)sender;
@end

@implementation CPTrayDelegate
- (void)handleMenuItem:(NSMenuItem *)sender {
    couchpilotTrayCommand((int)sender.tag);
}
@end

static NSMenuItem *cp_menu_item(NSString *title, NSInteger tag, CPTrayDelegate *delegate) {
    NSMenuItem *item = [[NSMenuItem alloc] initWithTitle:title action:@selector(handleMenuItem:) keyEquivalent:@""];
    item.target = delegate;
    item.tag = tag;
    return [item autorelease];
}

int cp_tray_start(const unsigned char *icon, int icon_length) {
    @autoreleasepool {
        if (![NSThread isMainThread] || cp_status_item != nil || icon == NULL || icon_length <= 0) return -1;
        NSApplication *application = NSApplication.sharedApplication;
        [application setActivationPolicy:NSApplicationActivationPolicyAccessory];

        // A command-line process does not go through NSApplicationMain, so AppKit
        // is not considered launched until we explicitly finish initialization.
        // Without this, the status item object exists but macOS may never publish
        // it to the menu bar.
        [application finishLaunching];

        NSImage *image = nil;
        BOOL image_owned = NO;
        if (@available(macOS 11.0, *)) {
            image = [NSImage imageWithSystemSymbolName:@"gamecontroller.fill"
                              accessibilityDescription:@"CouchPilot"];
            NSImageSymbolConfiguration *configuration =
                [NSImageSymbolConfiguration configurationWithPointSize:15
                                                                  weight:NSFontWeightSemibold];
            image = [image imageWithSymbolConfiguration:configuration];
        }
        if (image == nil) {
            NSData *data = [NSData dataWithBytes:icon length:(NSUInteger)icon_length];
            image = [[NSImage alloc] initWithData:data];
            image_owned = YES;
            image.size = NSMakeSize(18, 18);
        }
        if (image == nil) return -2;
        image.template = YES;

        // CGO compiles this translation unit without ARC. NSStatusBar does not
        // own the returned item for us, so retain it across the autorelease pool
        // and release it explicitly during shutdown.
        cp_status_item = [[NSStatusBar.systemStatusBar statusItemWithLength:NSVariableStatusItemLength] retain];
        if (cp_status_item == nil || cp_status_item.button == nil) return -3;
        cp_status_item.button.image = image;
        cp_status_item.button.imageScaling = NSImageScaleProportionallyDown;
        cp_status_item.button.imagePosition = NSImageLeft;
        cp_status_item.button.title = @"CP";
        cp_status_item.button.toolTip = @"CouchPilot 正在运行";
        if (image_owned) [image release];

        CPTrayDelegate *delegate = [[CPTrayDelegate alloc] init];
        cp_tray_delegate = delegate;
        NSMenu *menu = [[NSMenu alloc] initWithTitle:@"CouchPilot"];
        NSMenuItem *status = [[NSMenuItem alloc] initWithTitle:@"CouchPilot 正在运行" action:nil keyEquivalent:@""];
        status.enabled = NO;
        [menu addItem:status];
        [status release];
        [menu addItem:NSMenuItem.separatorItem];
        [menu addItem:cp_menu_item(@"打开日志", 1, delegate)];
        [menu addItem:cp_menu_item(@"打开配置目录", 2, delegate)];
        [menu addItem:NSMenuItem.separatorItem];
        [menu addItem:cp_menu_item(@"退出 CouchPilot", 3, delegate)];
        cp_status_item.menu = menu;
        [menu release];
        cp_log_tray_state("created");
        dispatch_after(dispatch_time(DISPATCH_TIME_NOW, (int64_t)(1 * NSEC_PER_SEC)),
                       dispatch_get_main_queue(), ^{
            cp_log_tray_state("after-1s");
        });
        dispatch_after(dispatch_time(DISPATCH_TIME_NOW, (int64_t)(3 * NSEC_PER_SEC)),
                       dispatch_get_main_queue(), ^{
            cp_log_tray_state("after-3s");
        });
        return 0;
    }
}

int cp_tray_run_main_loop(void) {
    @autoreleasepool {
        if (![NSThread isMainThread] || cp_status_item == nil) return -1;
        cp_log_tray_state("before-run");
        [NSApplication.sharedApplication run];
        cp_log_tray_state("after-run");
        cp_tray_cleanup();
        return 0;
    }
}

void cp_tray_dispose(void) {
    if ([NSThread isMainThread]) {
        cp_tray_cleanup();
    } else {
        dispatch_sync(dispatch_get_main_queue(), ^{ cp_tray_cleanup(); });
    }
}

void cp_tray_stop(void) {
    dispatch_async(dispatch_get_main_queue(), ^{
        NSApplication *application = NSApplication.sharedApplication;
        [application stop:nil];
        NSEvent *wake = [NSEvent otherEventWithType:NSEventTypeApplicationDefined
                                           location:NSZeroPoint
                                      modifierFlags:0
                                          timestamp:0
                                       windowNumber:0
                                            context:nil
                                            subtype:0
                                              data1:0
                                              data2:0];
        [application postEvent:wake atStart:NO];
    });
}
