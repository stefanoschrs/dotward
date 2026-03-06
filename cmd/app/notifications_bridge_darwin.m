#import <Foundation/Foundation.h>
#import <UserNotifications/UserNotifications.h>
#import <dispatch/dispatch.h>
#import <stdlib.h>
#import <string.h>

extern void HandleExtendAction(char *path);
extern void HandleUpdateAction(char *versionTag, char *publishedAt, char *appDownloadURL, char *cliDownloadURL);
extern void HandleSkipVersionAction(char *versionTag);

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
        } else if ([actionId isEqualToString:@"UPDATE_ACTION"]) {
            NSString *versionTag = response.notification.request.content.userInfo[@"version_tag"];
            NSString *publishedAt = response.notification.request.content.userInfo[@"published_at"];
            NSString *appDownloadURL = response.notification.request.content.userInfo[@"app_download_url"];
            NSString *cliDownloadURL = response.notification.request.content.userInfo[@"cli_download_url"];
            if (versionTag != nil && publishedAt != nil && appDownloadURL != nil && cliDownloadURL != nil) {
                HandleUpdateAction(strdup([versionTag UTF8String]),
                                   strdup([publishedAt UTF8String]),
                                   strdup([appDownloadURL UTF8String]),
                                   strdup([cliDownloadURL UTF8String]));
            }
        } else if ([actionId isEqualToString:@"SKIP_VERSION_ACTION"]) {
            NSString *versionTag = response.notification.request.content.userInfo[@"version_tag"];
            if (versionTag != nil) {
                HandleSkipVersionAction(strdup([versionTag UTF8String]));
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
        UNNotificationAction *updateAction = [UNNotificationAction actionWithIdentifier:@"UPDATE_ACTION"
                                                                                   title:@"Update"
                                                                                 options:UNNotificationActionOptionForeground];
        UNNotificationAction *skipVersionAction = [UNNotificationAction actionWithIdentifier:@"SKIP_VERSION_ACTION"
                                                                                        title:@"Skip this version"
                                                                                      options:UNNotificationActionOptionNone];
        UNNotificationAction *dismissAction = [UNNotificationAction actionWithIdentifier:@"DISMISS_UPDATE_ACTION"
                                                                                    title:@"Dismiss"
                                                                                  options:UNNotificationActionOptionNone];

        UNNotificationCategory *updateCategory = [UNNotificationCategory categoryWithIdentifier:@"UPDATE_AVAILABLE"
                                                                                         actions:@[updateAction, skipVersionAction, dismissAction]
                                                                               intentIdentifiers:@[]
                                                                                         options:UNNotificationCategoryOptionCustomDismissAction];
        [center setNotificationCategories:[NSSet setWithObjects:category, updateCategory, nil]];

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

int DotwardSendUpdateNotification(const char *version, const char *publishedAt, const char *appDownloadURL, const char *cliDownloadURL, const char *title, const char *body) {
    @autoreleasepool {
        if (version == NULL || publishedAt == NULL || appDownloadURL == NULL || cliDownloadURL == NULL || title == NULL || body == NULL) {
            return 0;
        }
        UNMutableNotificationContent *content = [UNMutableNotificationContent new];
        content.title = [NSString stringWithUTF8String:title];
        content.body = [NSString stringWithUTF8String:body];
        content.sound = [UNNotificationSound defaultSound];
        content.categoryIdentifier = @"UPDATE_AVAILABLE";

        NSString *versionStr = [NSString stringWithUTF8String:version];
        content.userInfo = @{
            @"version_tag": versionStr,
            @"published_at": [NSString stringWithUTF8String:publishedAt],
            @"app_download_url": [NSString stringWithUTF8String:appDownloadURL],
            @"cli_download_url": [NSString stringWithUTF8String:cliDownloadURL]
        };

        NSString *identifier = DotwardIdentifier(@"update-available", versionStr);
        return DotwardSendNotification(identifier, content);
    }
}
