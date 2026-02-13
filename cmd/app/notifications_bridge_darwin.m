#import <Foundation/Foundation.h>
#import <UserNotifications/UserNotifications.h>
#import <dispatch/dispatch.h>
#import <stdlib.h>
#import <string.h>

extern void HandleExtendAction(char *path);

@interface DotwardDelegate : NSObject <UNUserNotificationCenterDelegate>
@end

@implementation DotwardDelegate

- (void)userNotificationCenter:(UNUserNotificationCenter *)center
didReceiveNotificationResponse:(UNNotificationResponse *)response
         withCompletionHandler:(void (^)(void))completionHandler
{
    @autoreleasepool {
        NSString *actionId = response.actionIdentifier;
        if ([actionId isEqualToString:@"EXTEND_ACTION"]) {
            NSString *path = response.notification.request.content.userInfo[@"path"];
            if (path != nil) {
                const char *utf8 = [path UTF8String];
                char *copy = strdup(utf8);
                HandleExtendAction(copy);
            }
        }
        completionHandler();
    }
}

- (void)userNotificationCenter:(UNUserNotificationCenter *)center
       willPresentNotification:(UNNotification *)notification
         withCompletionHandler:(void (^)(UNNotificationPresentationOptions options))completionHandler
{
    completionHandler(UNNotificationPresentationOptionBanner | UNNotificationPresentationOptionSound);
}

@end

static DotwardDelegate *dotwardDelegate;

void DotwardInitNotifications(void) {
    @autoreleasepool {
        UNUserNotificationCenter *center = [UNUserNotificationCenter currentNotificationCenter];

        UNNotificationAction *extendAction = [UNNotificationAction actionWithIdentifier:@"EXTEND_ACTION"
                                                                                   title:@"Extend 1 Hour"
                                                                                 options:UNNotificationActionOptionNone];

        UNNotificationCategory *category = [UNNotificationCategory categoryWithIdentifier:@"EXPIRY_WARNING"
                                                                                  actions:@[extendAction]
                                                                        intentIdentifiers:@[]
                                                                                  options:UNNotificationCategoryOptionCustomDismissAction];
        [center setNotificationCategories:[NSSet setWithObject:category]];

        dotwardDelegate = [DotwardDelegate new];
        [center setDelegate:dotwardDelegate];

        [center requestAuthorizationWithOptions:(UNAuthorizationOptionAlert | UNAuthorizationOptionSound)
                              completionHandler:^(BOOL granted, NSError * _Nullable error) {
            (void)granted;
            (void)error;
        }];
    }
}

static BOOL DotwardNotificationsAuthorized(void) {
    __block BOOL allowed = NO;
    dispatch_semaphore_t sem = dispatch_semaphore_create(0);
    [[UNUserNotificationCenter currentNotificationCenter]
        getNotificationSettingsWithCompletionHandler:^(UNNotificationSettings *settings) {
            if (settings.authorizationStatus == UNAuthorizationStatusAuthorized ||
                settings.authorizationStatus == UNAuthorizationStatusProvisional) {
                allowed = YES;
            }
            dispatch_semaphore_signal(sem);
        }];
    dispatch_time_t timeout = dispatch_time(DISPATCH_TIME_NOW, (int64_t)(1 * NSEC_PER_SEC));
    long waitResult = dispatch_semaphore_wait(sem, timeout);
    if (waitResult != 0) {
        return NO;
    }
    return allowed;
}

static NSString *DotwardIdentifier(NSString *prefix, NSString *path) {
    NSCharacterSet *allowed = [NSCharacterSet alphanumericCharacterSet];
    NSString *sanitized = [[path componentsSeparatedByCharactersInSet:[allowed invertedSet]]
        componentsJoinedByString:@"_"];
    if (sanitized.length == 0) {
        sanitized = @"unknown";
    }
    if (sanitized.length > 96) {
        sanitized = [sanitized substringToIndex:96];
    }
    return [NSString stringWithFormat:@"%@-%@", prefix, sanitized];
}

static int DotwardSendNotification(NSString *identifier, UNMutableNotificationContent *content) {
    if (!DotwardNotificationsAuthorized()) {
        return 0;
    }

    __block BOOL ok = YES;
    dispatch_semaphore_t sem = dispatch_semaphore_create(0);
    UNTimeIntervalNotificationTrigger *trigger =
        [UNTimeIntervalNotificationTrigger triggerWithTimeInterval:1 repeats:NO];
    UNNotificationRequest *request =
        [UNNotificationRequest requestWithIdentifier:identifier content:content trigger:trigger];

    [[UNUserNotificationCenter currentNotificationCenter]
        addNotificationRequest:request
          withCompletionHandler:^(NSError * _Nullable error) {
              if (error != nil) {
                  ok = NO;
              }
              dispatch_semaphore_signal(sem);
          }];

    dispatch_time_t timeout = dispatch_time(DISPATCH_TIME_NOW, (int64_t)(1 * NSEC_PER_SEC));
    long waitResult = dispatch_semaphore_wait(sem, timeout);
    if (waitResult != 0) {
        return 0;
    }
    return ok ? 1 : 0;
}

int DotwardSendExpiryNotification(const char *path, const char *title, const char *body) {
    @autoreleasepool {
        if (path == NULL || title == NULL || body == NULL) {
            return 0;
        }
        UNMutableNotificationContent *content = [UNMutableNotificationContent new];
        content.title = [NSString stringWithUTF8String:title];
        content.body = [NSString stringWithUTF8String:body];
        content.sound = [UNNotificationSound defaultSound];
        content.categoryIdentifier = @"EXPIRY_WARNING";

        NSString *pathStr = [NSString stringWithUTF8String:path];
        content.userInfo = @{ @"path": pathStr };

        NSString *identifier = DotwardIdentifier(@"expiry-warning", pathStr);
        return DotwardSendNotification(identifier, content);
    }
}

int DotwardSendDeletedNotification(const char *path, const char *title, const char *body) {
    @autoreleasepool {
        if (path == NULL || title == NULL || body == NULL) {
            return 0;
        }
        UNMutableNotificationContent *content = [UNMutableNotificationContent new];
        content.title = [NSString stringWithUTF8String:title];
        content.body = [NSString stringWithUTF8String:body];
        content.sound = [UNNotificationSound defaultSound];

        NSString *pathStr = [NSString stringWithUTF8String:path];
        NSString *identifier = DotwardIdentifier(@"deleted", pathStr);
        return DotwardSendNotification(identifier, content);
    }
}
