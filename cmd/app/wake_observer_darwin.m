#import <AppKit/AppKit.h>
#import <Foundation/Foundation.h>

extern void HandleSystemWake(void);

static id dotwardWakeObserver;

void DotwardRegisterWakeObserver(void) {
    @autoreleasepool {
        NSNotificationCenter *center = [[NSWorkspace sharedWorkspace] notificationCenter];
        if (dotwardWakeObserver != nil) {
            [center removeObserver:dotwardWakeObserver];
            dotwardWakeObserver = nil;
        }
        dotwardWakeObserver = [center addObserverForName:NSWorkspaceDidWakeNotification
                                                  object:nil
                                                   queue:[NSOperationQueue mainQueue]
                                              usingBlock:^(__unused NSNotification *note) {
            HandleSystemWake();
        }];
    }
}
