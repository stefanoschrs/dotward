#import <AppKit/AppKit.h>
#import <Foundation/Foundation.h>

extern void HandleSystemWake(void);

static id dotwardWakeObserver;
static id dotwardClockObserver;
static id dotwardDayObserver;

void DotwardRegisterRuntimeObservers(void) {
    @autoreleasepool {
        NSNotificationCenter *workspaceCenter = [[NSWorkspace sharedWorkspace] notificationCenter];
        NSNotificationCenter *defaultCenter = [NSNotificationCenter defaultCenter];

        if (dotwardWakeObserver != nil) {
            [workspaceCenter removeObserver:dotwardWakeObserver];
            dotwardWakeObserver = nil;
        }
        if (dotwardClockObserver != nil) {
            [defaultCenter removeObserver:dotwardClockObserver];
            dotwardClockObserver = nil;
        }
        if (dotwardDayObserver != nil) {
            [defaultCenter removeObserver:dotwardDayObserver];
            dotwardDayObserver = nil;
        }

        dotwardWakeObserver = [workspaceCenter addObserverForName:NSWorkspaceDidWakeNotification
                                                           object:nil
                                                            queue:[NSOperationQueue mainQueue]
                                                       usingBlock:^(__unused NSNotification *note) {
            HandleSystemWake();
        }];
        dotwardClockObserver = [defaultCenter addObserverForName:NSSystemClockDidChangeNotification
                                                          object:nil
                                                           queue:[NSOperationQueue mainQueue]
                                                      usingBlock:^(__unused NSNotification *note) {
            HandleSystemWake();
        }];
        dotwardDayObserver = [defaultCenter addObserverForName:NSCalendarDayChangedNotification
                                                        object:nil
                                                         queue:[NSOperationQueue mainQueue]
                                                    usingBlock:^(__unused NSNotification *note) {
            HandleSystemWake();
        }];
    }
}
