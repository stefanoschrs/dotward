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
                                                                                   title:@"Extend Session"
                                                                                 options:UNNotificationActionOptionNone];

        UNNotificationCategory *category = [UNNotificationCategory categoryWithIdentifier:@"EXPIRY_WARNING"
                                                                                  actions:@[extendAction]
                                                                        intentIdentifiers:@[]
                                                                                  options:UNNotificationCategoryOptionCustomDismissAction];
        [center setNotificationCategories:[NSSet setWithObject:category]];

        dotwardDelegate = [DotwardDelegate new];
        [center setDelegate:dotwardDelegate];

        dispatch_async(dispatch_get_main_queue(), ^{
            [center requestAuthorizationWithOptions:(UNAuthorizationOptionAlert | UNAuthorizationOptionSound | UNAuthorizationOptionBadge)
                                  completionHandler:^(BOOL granted, NSError * _Nullable error) {
                (void)error;
                if (granted) {
                    UNMutableNotificationContent *content = [UNMutableNotificationContent new];
                    content.title = @"Dotward Ready";
                    content.body = @"Notifications are enabled.";
                    content.sound = [UNNotificationSound defaultSound];
                    UNTimeIntervalNotificationTrigger *trigger =
                        [UNTimeIntervalNotificationTrigger triggerWithTimeInterval:1 repeats:NO];
                    UNNotificationRequest *request =
                        [UNNotificationRequest requestWithIdentifier:@"dotward-startup" content:content trigger:trigger];
                    [[UNUserNotificationCenter currentNotificationCenter] addNotificationRequest:request withCompletionHandler:nil];
                }
            }];
        });
    }
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
    UNTimeIntervalNotificationTrigger *trigger =
        [UNTimeIntervalNotificationTrigger triggerWithTimeInterval:1 repeats:NO];
    UNNotificationRequest *request =
        [UNNotificationRequest requestWithIdentifier:identifier content:content trigger:trigger];

    [[UNUserNotificationCenter currentNotificationCenter]
        addNotificationRequest:request
          withCompletionHandler:^(NSError * _Nullable error) {
              (void)error;
          }];
    return 1;
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

int DotwardSendUnlockedNotification(const char *path, const char *title, const char *body) {
    @autoreleasepool {
        if (path == NULL || title == NULL || body == NULL) {
            return 0;
        }
        UNMutableNotificationContent *content = [UNMutableNotificationContent new];
        content.title = [NSString stringWithUTF8String:title];
        content.body = [NSString stringWithUTF8String:body];
        content.sound = [UNNotificationSound defaultSound];

        NSString *pathStr = [NSString stringWithUTF8String:path];
        NSString *identifier = DotwardIdentifier(@"unlocked", pathStr);
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
