//go:build darwin

#import <AppKit/AppKit.h>
#import <CoreGraphics/CoreGraphics.h>
#import <GameController/GameController.h>
#import <IOKit/hid/IOHIDLib.h>
#import <IOKit/hid/IOHIDKeys.h>
#import <IOKit/hidsystem/ev_keymap.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <math.h>
#include <pthread.h>
#include "native.h"

static IOHIDManagerRef cp_hid_manager = NULL;
static CFMutableArrayRef cp_hid_devices = NULL;
static CFMutableDictionaryRef cp_hid_element_cache = NULL;
static dispatch_queue_t cp_hid_queue = NULL;
static pthread_mutex_t cp_hid_devices_lock = PTHREAD_MUTEX_INITIALIZER;
static int cp_hid_initialize_result = -1;
static uint64_t cp_gamepad_token_secret = 0;

static uint64_t cp_gamepad_token(const void *device) {
    uint64_t value = (uint64_t)(uintptr_t)device ^ cp_gamepad_token_secret;
    value ^= value >> 30;
    value *= UINT64_C(0xbf58476d1ce4e5b9);
    value ^= value >> 27;
    value *= UINT64_C(0x94d049bb133111eb);
    value ^= value >> 31;
    return value == 0 ? 1 : value;
}

static uint64_t cp_hid_numeric_property(IOHIDDeviceRef device, CFStringRef key) {
    CFTypeRef value = IOHIDDeviceGetProperty(device, key);
    if (value == NULL || CFGetTypeID(value) != CFNumberGetTypeID()) return 0;
    uint64_t number = 0;
    CFNumberGetValue((CFNumberRef)value, kCFNumberSInt64Type, &number);
    return number;
}

static BOOL cp_hid_is_gamecontroller_synthetic(IOHIDDeviceRef device) {
    if (device == NULL) return NO;
    if (IOHIDDeviceGetProperty(device, CFSTR("_GCSyntheticDeviceIdentifier")) != NULL ||
        IOHIDDeviceGetProperty(device, CFSTR("_GCSyntheticDeviceControllerIdentifier")) != NULL) return YES;

    CFTypeRef marker = IOHIDDeviceGetProperty(device, CFSTR("GCSyntheticDevice"));
    if (marker == NULL) return NO;
    if (CFGetTypeID(marker) == CFBooleanGetTypeID()) return CFBooleanGetValue((CFBooleanRef)marker);
    if (CFGetTypeID(marker) == CFNumberGetTypeID()) {
        int value = 0;
        return CFNumberGetValue((CFNumberRef)marker, kCFNumberIntType, &value) && value != 0;
    }
    if (CFGetTypeID(marker) == CFStringGetTypeID()) {
        CFStringRef value = (CFStringRef)marker;
        return CFStringCompare(value, CFSTR("yes"), kCFCompareCaseInsensitive) == kCFCompareEqualTo ||
            CFStringCompare(value, CFSTR("true"), kCFCompareCaseInsensitive) == kCFCompareEqualTo ||
            CFStringCompare(value, CFSTR("1"), 0) == kCFCompareEqualTo;
    }
    return NO;
}

static int cp_hid_device_order(IOHIDDeviceRef left, IOHIDDeviceRef right) {
    uint64_t left_location = cp_hid_numeric_property(left, CFSTR(kIOHIDLocationIDKey));
    uint64_t right_location = cp_hid_numeric_property(right, CFSTR(kIOHIDLocationIDKey));
    if (left_location < right_location) return -1;
    if (left_location > right_location) return 1;
    uint64_t left_vendor = cp_hid_numeric_property(left, CFSTR(kIOHIDVendorIDKey));
    uint64_t right_vendor = cp_hid_numeric_property(right, CFSTR(kIOHIDVendorIDKey));
    if (left_vendor < right_vendor) return -1;
    if (left_vendor > right_vendor) return 1;
    uint64_t left_product = cp_hid_numeric_property(left, CFSTR(kIOHIDProductIDKey));
    uint64_t right_product = cp_hid_numeric_property(right, CFSTR(kIOHIDProductIDKey));
    if (left_product < right_product) return -1;
    if (left_product > right_product) return 1;
    uintptr_t left_address = (uintptr_t)left;
    uintptr_t right_address = (uintptr_t)right;
    return left_address < right_address ? -1 : left_address > right_address ? 1 : 0;
}

static CFComparisonResult cp_hid_compare_devices(const void *left_value, const void *right_value, void *context) {
    (void)context;
    int order = cp_hid_device_order((IOHIDDeviceRef)left_value, (IOHIDDeviceRef)right_value);
    return order < 0 ? kCFCompareLessThan : order > 0 ? kCFCompareGreaterThan : kCFCompareEqualTo;
}

static CFMutableDictionaryRef cp_hid_match(int usage) {
    CFMutableDictionaryRef match = CFDictionaryCreateMutable(
        kCFAllocatorDefault, 2, &kCFTypeDictionaryKeyCallBacks,
        &kCFTypeDictionaryValueCallBacks);
    int page = kHIDPage_GenericDesktop;
    CFNumberRef page_number = CFNumberCreate(kCFAllocatorDefault, kCFNumberIntType, &page);
    CFNumberRef usage_number = CFNumberCreate(kCFAllocatorDefault, kCFNumberIntType, &usage);
    CFDictionarySetValue(match, CFSTR(kIOHIDDeviceUsagePageKey), page_number);
    CFDictionarySetValue(match, CFSTR(kIOHIDDeviceUsageKey), usage_number);
    CFRelease(page_number);
    CFRelease(usage_number);
    return match;
}

static void cp_hid_add_device(IOHIDDeviceRef device) {
    if (device == NULL) return;
    pthread_mutex_lock(&cp_hid_devices_lock);
    CFIndex count = CFArrayGetCount(cp_hid_devices);
    for (CFIndex index = 0; index < count; index++) {
        if (CFArrayGetValueAtIndex(cp_hid_devices, index) == device) {
            pthread_mutex_unlock(&cp_hid_devices_lock);
            return;
        }
    }
    CFArrayRef elements = IOHIDDeviceCopyMatchingElements(device, NULL, kIOHIDOptionsTypeNone);
    if (elements == NULL) {
        pthread_mutex_unlock(&cp_hid_devices_lock);
        return;
    }
    CFDictionarySetValue(cp_hid_element_cache, device, elements);
    CFRelease(elements);
    CFArrayAppendValue(cp_hid_devices, device);
    CFArraySortValues(cp_hid_devices, CFRangeMake(0, count + 1), cp_hid_compare_devices, NULL);
    pthread_mutex_unlock(&cp_hid_devices_lock);
}

static void cp_hid_device_matched(void *context, IOReturn result, void *sender, IOHIDDeviceRef device) {
    (void)context;
    (void)sender;
    if (result == kIOReturnSuccess) cp_hid_add_device(device);
}

static void cp_hid_device_removed(void *context, IOReturn result, void *sender, IOHIDDeviceRef device) {
    (void)context;
    (void)result;
    (void)sender;
    if (device == NULL) return;
    pthread_mutex_lock(&cp_hid_devices_lock);
    CFIndex count = CFArrayGetCount(cp_hid_devices);
    for (CFIndex index = 0; index < count; index++) {
        if (CFArrayGetValueAtIndex(cp_hid_devices, index) == device) {
            CFDictionaryRemoveValue(cp_hid_element_cache, device);
            CFArrayRemoveValueAtIndex(cp_hid_devices, index);
            break;
        }
    }
    pthread_mutex_unlock(&cp_hid_devices_lock);
}

static int cp_hid_visible_count(void) {
    pthread_mutex_lock(&cp_hid_devices_lock);
    int visible = 0;
    if (cp_hid_devices != NULL) {
        CFIndex count = CFArrayGetCount(cp_hid_devices);
        for (CFIndex index = 0; index < count; index++) {
            IOHIDDeviceRef device = (IOHIDDeviceRef)CFArrayGetValueAtIndex(cp_hid_devices, index);
            if (!cp_hid_is_gamecontroller_synthetic(device)) visible++;
        }
    }
    pthread_mutex_unlock(&cp_hid_devices_lock);
    return visible;
}

static IOHIDDeviceRef cp_hid_visible_device_at(int wanted) {
    pthread_mutex_lock(&cp_hid_devices_lock);
    IOHIDDeviceRef device = NULL;
    if (cp_hid_devices != NULL && wanted >= 0) {
        int visible = 0;
        CFIndex count = CFArrayGetCount(cp_hid_devices);
        for (CFIndex index = 0; index < count; index++) {
            IOHIDDeviceRef candidate = (IOHIDDeviceRef)CFArrayGetValueAtIndex(cp_hid_devices, index);
            if (cp_hid_is_gamecontroller_synthetic(candidate)) continue;
            if (visible == wanted) {
                device = candidate;
                CFRetain(device);
                break;
            }
            visible++;
        }
    }
    pthread_mutex_unlock(&cp_hid_devices_lock);
    return device;
}

static IOHIDDeviceRef cp_hid_device_for_token(uint64_t token) {
    pthread_mutex_lock(&cp_hid_devices_lock);
    IOHIDDeviceRef device = NULL;
    if (cp_hid_devices != NULL) {
        CFIndex count = CFArrayGetCount(cp_hid_devices);
        for (CFIndex index = 0; index < count; index++) {
            IOHIDDeviceRef candidate = (IOHIDDeviceRef)CFArrayGetValueAtIndex(cp_hid_devices, index);
            if (cp_gamepad_token(candidate) == token) {
                device = candidate;
                CFRetain(device);
                break;
            }
        }
    }
    pthread_mutex_unlock(&cp_hid_devices_lock);
    return device;
}

static CFArrayRef cp_hid_elements_for_device(IOHIDDeviceRef device) {
    pthread_mutex_lock(&cp_hid_devices_lock);
    CFArrayRef elements = NULL;
    if (device != NULL && cp_hid_element_cache != NULL) {
        elements = (CFArrayRef)CFDictionaryGetValue(cp_hid_element_cache, device);
        if (elements != NULL) CFRetain(elements);
    }
    pthread_mutex_unlock(&cp_hid_devices_lock);
    return elements;
}

static int cp_hid_initialize(void) {
    if (cp_hid_manager != NULL) return cp_hid_initialize_result;
    cp_hid_devices = CFArrayCreateMutable(kCFAllocatorDefault, 0, &kCFTypeArrayCallBacks);
    cp_hid_element_cache = CFDictionaryCreateMutable(
        kCFAllocatorDefault, 0, &kCFTypeDictionaryKeyCallBacks,
        &kCFTypeDictionaryValueCallBacks);
    cp_hid_manager = IOHIDManagerCreate(kCFAllocatorDefault, kIOHIDOptionsTypeNone);
    if (cp_hid_manager == NULL) {
        cp_hid_initialize_result = -2;
        return cp_hid_initialize_result;
    }
    CFMutableDictionaryRef joystick = cp_hid_match(kHIDUsage_GD_Joystick);
    CFMutableDictionaryRef gamepad = cp_hid_match(kHIDUsage_GD_GamePad);
    const void *matches[] = { joystick, gamepad };
    CFArrayRef match_array = CFArrayCreate(kCFAllocatorDefault, matches, 2, &kCFTypeArrayCallBacks);
    IOHIDManagerSetDeviceMatchingMultiple(cp_hid_manager, match_array);
    IOHIDManagerRegisterDeviceMatchingCallback(cp_hid_manager, cp_hid_device_matched, NULL);
    IOHIDManagerRegisterDeviceRemovalCallback(cp_hid_manager, cp_hid_device_removed, NULL);
    cp_hid_queue = dispatch_queue_create("io.github.couchpilot.hid", DISPATCH_QUEUE_SERIAL);
    IOHIDManagerSetDispatchQueue(cp_hid_manager, cp_hid_queue);
    IOReturn open_result = IOHIDManagerOpen(cp_hid_manager, kIOHIDOptionsTypeNone);
    if (open_result != kIOReturnSuccess) {
        cp_hid_initialize_result = (int)open_result;
        CFRelease(match_array);
        CFRelease(joystick);
        CFRelease(gamepad);
        return cp_hid_initialize_result;
    }

    // Prime already-connected receivers synchronously; future add/remove events
    // arrive on the dedicated queue even when a diagnostic CLI owns main.
    CFSetRef devices = IOHIDManagerCopyDevices(cp_hid_manager);
    if (devices != NULL) {
        CFIndex count = CFSetGetCount(devices);
        IOHIDDeviceRef *values = calloc((size_t)count, sizeof(IOHIDDeviceRef));
        if (values != NULL) {
            CFSetGetValues(devices, (const void **)values);
            for (CFIndex index = 0; index < count; index++) cp_hid_add_device(values[index]);
            free(values);
        }
        CFRelease(devices);
    }
    IOHIDManagerActivate(cp_hid_manager);
    CFRelease(match_array);
    CFRelease(joystick);
    CFRelease(gamepad);
    cp_hid_initialize_result = 0;
    return 0;
}

static int cp_gc_count(void) {
    int count = 0;
    for (GCController *controller in GCController.controllers) {
        if (controller.extendedGamepad != nil) count++;
    }
    return count;
}

int cp_gamepad_initialize(void) {
    @autoreleasepool {
        if (cp_gamepad_token_secret == 0) {
            arc4random_buf(&cp_gamepad_token_secret, sizeof(cp_gamepad_token_secret));
            if (cp_gamepad_token_secret == 0) cp_gamepad_token_secret = UINT64_C(0x9e3779b97f4a7c15);
        }
        if (@available(macOS 11.3, *)) {
            // CouchPilot is an LSUIElement/background agent by design. Since
            // Big Sur 11.3 GameController otherwise suppresses all input while
            // another application is frontmost.
            GCController.shouldMonitorBackgroundEvents = YES;
        }
        // IOHID is a fallback for receivers that GameController does not
        // recognize. Its failure must not disable an otherwise usable primary
        // GameController backend.
        int hid_result = cp_hid_initialize();
        if (hid_result != 0) {
            fprintf(stderr, "warning: IOHID gamepad fallback unavailable (%d)\n", hid_result);
        }
        return 0;
    }
}

int cp_gamepad_count(void) {
    @autoreleasepool {
        return cp_gc_count() + cp_hid_visible_count();
    }
}

static GCController *cp_controller_at(int wanted) {
    int index = 0;
    for (GCController *controller in GCController.controllers) {
        if (controller.extendedGamepad == nil) continue;
        if (index == wanted) return controller;
        index++;
    }
    return nil;
}

static GCController *cp_controller_for_token(uint64_t token) {
    for (GCController *controller in GCController.controllers) {
        if (controller.extendedGamepad != nil && cp_gamepad_token(controller) == token) return controller;
    }
    return nil;
}

int cp_gamepad_device_at(int wanted, int *backend, uint64_t *token) {
    if (wanted < 0 || backend == NULL || token == NULL) return -1;
    @autoreleasepool {
        int gc_count = cp_gc_count();
        if (wanted < gc_count) {
            GCController *controller = cp_controller_at(wanted);
            if (controller == nil) return 0;
            *backend = CP_GAMEPAD_BACKEND_GAMECONTROLLER;
            *token = cp_gamepad_token(controller);
            return 1;
        }
        IOHIDDeviceRef device = cp_hid_visible_device_at(wanted - gc_count);
        if (device == NULL) return 0;
        *backend = CP_GAMEPAD_BACKEND_HID;
        *token = cp_gamepad_token(device);
        CFRelease(device);
        return 1;
    }
}

static BOOL cp_pressed(GCControllerButtonInput *button) {
    return button != nil && button.isPressed;
}

static double cp_hid_normalized(IOHIDElementRef element, CFIndex value, double low, double high) {
    CFIndex minimum = IOHIDElementGetLogicalMin(element);
    CFIndex maximum = IOHIDElementGetLogicalMax(element);
    if (maximum <= minimum) return 0;
    double unit = ((double)value - (double)minimum) / ((double)maximum - (double)minimum);
    return low + unit * (high - low);
}

static BOOL cp_hid_is_xbox(IOHIDDeviceRef device) {
    CFTypeRef user_class = IOHIDDeviceGetProperty(device, CFSTR("IOUserClass"));
    CFTypeRef product = IOHIDDeviceGetProperty(device, CFSTR(kIOHIDProductKey));
    if (user_class != NULL && CFGetTypeID(user_class) == CFStringGetTypeID() &&
        CFStringFind((CFStringRef)user_class, CFSTR("Xbox"), kCFCompareCaseInsensitive).location != kCFNotFound) return YES;
    if (product != NULL && CFGetTypeID(product) == CFStringGetTypeID() &&
        CFStringFind((CFStringRef)product, CFSTR("XINPUT"), kCFCompareCaseInsensitive).location != kCFNotFound) return YES;
    return NO;
}

static void cp_hid_button(cp_gamepad_state *state, uint32_t usage, BOOL xbox) {
    uint16_t bit = 0;
    if (xbox) {
        switch (usage) {
            case 1: bit = CP_BUTTON_A; break; case 2: bit = CP_BUTTON_B; break;
            case 3: bit = CP_BUTTON_X; break; case 4: bit = CP_BUTTON_Y; break;
            case 5: bit = CP_BUTTON_LEFT_SHOULDER; break; case 6: bit = CP_BUTTON_RIGHT_SHOULDER; break;
            // Apple's XboxGamepad DEXT exposes the XInput bit order through
            // HID usages 9/10/7/8 for Start/Back/L3/R3 respectively.
            case 7: bit = CP_BUTTON_LEFT_THUMB; break; case 8: bit = CP_BUTTON_RIGHT_THUMB; break;
            case 9: bit = CP_BUTTON_START; break; case 10: bit = CP_BUTTON_BACK; break;
            case 12: bit = CP_BUTTON_DPAD_UP; break; case 13: bit = CP_BUTTON_DPAD_DOWN; break;
            case 14: bit = CP_BUTTON_DPAD_LEFT; break; case 15: bit = CP_BUTTON_DPAD_RIGHT; break;
        }
    } else {
        switch (usage) {
            case 1: bit = CP_BUTTON_A; break; case 2: bit = CP_BUTTON_B; break;
            case 3: bit = CP_BUTTON_X; break; case 4: bit = CP_BUTTON_Y; break;
            case 5: bit = CP_BUTTON_LEFT_SHOULDER; break; case 6: bit = CP_BUTTON_RIGHT_SHOULDER; break;
            case 7: bit = CP_BUTTON_BACK; break; case 8: bit = CP_BUTTON_START; break;
            case 9: bit = CP_BUTTON_LEFT_THUMB; break; case 10: bit = CP_BUTTON_RIGHT_THUMB; break;
        }
    }
    state->buttons |= bit;
}

static int cp_hid_read(uint64_t token, cp_gamepad_state *state) {
    IOHIDDeviceRef device = cp_hid_device_for_token(token);
    if (device == NULL) return 0;
    CFArrayRef elements = cp_hid_elements_for_device(device);
    if (elements == NULL) {
        CFRelease(device);
        return 0;
    }
    BOOL xbox = cp_hid_is_xbox(device);
    memset(state, 0, sizeof(*state));
    BOOL read_any_value = NO;
    CFIndex count = CFArrayGetCount(elements);
    for (CFIndex item = 0; item < count; item++) {
        IOHIDElementRef element = (IOHIDElementRef)CFArrayGetValueAtIndex(elements, item);
        IOHIDElementType type = IOHIDElementGetType(element);
        if (type != kIOHIDElementTypeInput_Misc && type != kIOHIDElementTypeInput_Button &&
            type != kIOHIDElementTypeInput_Axis) continue;
        IOHIDValueRef hid_value = NULL;
        if (IOHIDDeviceGetValue(device, element, &hid_value) != kIOReturnSuccess || hid_value == NULL) continue;
        read_any_value = YES;
        CFIndex value = IOHIDValueGetIntegerValue(hid_value);
        uint32_t page = IOHIDElementGetUsagePage(element);
        uint32_t usage = IOHIDElementGetUsage(element);
        if (page == kHIDPage_Button && value != 0) {
            cp_hid_button(state, usage, xbox);
        } else if (page == kHIDPage_GenericDesktop) {
            switch (usage) {
                case kHIDUsage_GD_X: state->left_x = (float)cp_hid_normalized(element, value, -1, 1); break;
                case kHIDUsage_GD_Y: state->left_y = (float)cp_hid_normalized(element, value, -1, 1); break;
                case kHIDUsage_GD_Rx: state->right_x = (float)cp_hid_normalized(element, value, -1, 1); break;
                case kHIDUsage_GD_Ry: state->right_y = (float)cp_hid_normalized(element, value, -1, 1); break;
                case kHIDUsage_GD_Z: state->left_trigger = (float)cp_hid_normalized(element, value, 0, 1); break;
                case kHIDUsage_GD_Rz: state->right_trigger = (float)cp_hid_normalized(element, value, 0, 1); break;
                case kHIDUsage_GD_Hatswitch: {
                    int hat = (int)value - (int)IOHIDElementGetLogicalMin(element);
                    if (hat == 0 || hat == 1 || hat == 7) state->buttons |= CP_BUTTON_DPAD_UP;
                    if (hat == 3 || hat == 4 || hat == 5) state->buttons |= CP_BUTTON_DPAD_DOWN;
                    if (hat == 5 || hat == 6 || hat == 7) state->buttons |= CP_BUTTON_DPAD_LEFT;
                    if (hat == 1 || hat == 2 || hat == 3) state->buttons |= CP_BUTTON_DPAD_RIGHT;
                    break;
                }
            }
        }
    }
    CFRelease(elements);
    CFRelease(device);
    if (!read_any_value) {
        return 0;
    }
    return 1;
}

int cp_gamepad_diagnostic(int backend, uint64_t token, char *buffer, int length) {
    if (buffer == NULL || length < 2) return -1;
    buffer[0] = '\0';
    @autoreleasepool {
        if (backend == CP_GAMEPAD_BACKEND_GAMECONTROLLER) {
            if (cp_controller_for_token(token) == nil) return -1;
            snprintf(buffer, (size_t)length, "GameController framework input");
            return (int)strlen(buffer);
        }
        if (backend != CP_GAMEPAD_BACKEND_HID) return -1;
        IOHIDDeviceRef device = cp_hid_device_for_token(token);
        if (device == NULL) return -1;
        CFArrayRef elements = cp_hid_elements_for_device(device);
        if (elements == NULL) {
            CFRelease(device);
            return -1;
        }
        int used = snprintf(buffer, (size_t)length, "xbox=%d", cp_hid_is_xbox(device) ? 1 : 0);
        CFIndex count = CFArrayGetCount(elements);
        for (CFIndex item = 0; item < count && used < length - 1; item++) {
            IOHIDElementRef element = (IOHIDElementRef)CFArrayGetValueAtIndex(elements, item);
            IOHIDElementType type = IOHIDElementGetType(element);
            if (type != kIOHIDElementTypeInput_Misc && type != kIOHIDElementTypeInput_Button &&
                type != kIOHIDElementTypeInput_Axis) continue;
            IOHIDValueRef hid_value = NULL;
            if (IOHIDDeviceGetValue(device, element, &hid_value) != kIOReturnSuccess || hid_value == NULL) continue;
            CFIndex value = IOHIDValueGetIntegerValue(hid_value);
            uint32_t page = IOHIDElementGetUsagePage(element);
            uint32_t usage = IOHIDElementGetUsage(element);
            if (page == kHIDPage_Button && value == 0) continue;
            if (page != kHIDPage_Button && !(page == kHIDPage_GenericDesktop &&
                (usage == kHIDUsage_GD_X || usage == kHIDUsage_GD_Y || usage == kHIDUsage_GD_Z ||
                 usage == kHIDUsage_GD_Rx || usage == kHIDUsage_GD_Ry || usage == kHIDUsage_GD_Rz ||
                 usage == kHIDUsage_GD_Hatswitch))) continue;
            int written = snprintf(buffer + used, (size_t)(length - used), " p%u:u%u=%ld", page, usage, (long)value);
            if (written < 0) break;
            used += written;
        }
        CFRelease(elements);
        CFRelease(device);
        buffer[length - 1] = '\0';
        return (int)strlen(buffer);
    }
}

int cp_gamepad_read(int backend, uint64_t token, cp_gamepad_state *state) {
    if (state == NULL || token == 0) return -1;
    @autoreleasepool {
        if (backend == CP_GAMEPAD_BACKEND_HID) return cp_hid_read(token, state);
        if (backend != CP_GAMEPAD_BACKEND_GAMECONTROLLER) return -1;
        GCController *controller = cp_controller_for_token(token);
        if (controller == nil) return 0;
        GCController *snapshot = [controller capture];
        GCExtendedGamepad *pad = snapshot.extendedGamepad;
        if (pad == nil) return 0;
        uint16_t buttons = 0;
        if (cp_pressed(pad.dpad.up)) buttons |= CP_BUTTON_DPAD_UP;
        if (cp_pressed(pad.dpad.down)) buttons |= CP_BUTTON_DPAD_DOWN;
        if (cp_pressed(pad.dpad.left)) buttons |= CP_BUTTON_DPAD_LEFT;
        if (cp_pressed(pad.dpad.right)) buttons |= CP_BUTTON_DPAD_RIGHT;
        if (cp_pressed(pad.buttonMenu)) buttons |= CP_BUTTON_START;
        if (cp_pressed(pad.buttonOptions)) buttons |= CP_BUTTON_BACK;
        if (cp_pressed(pad.leftThumbstickButton)) buttons |= CP_BUTTON_LEFT_THUMB;
        if (cp_pressed(pad.rightThumbstickButton)) buttons |= CP_BUTTON_RIGHT_THUMB;
        if (cp_pressed(pad.leftShoulder)) buttons |= CP_BUTTON_LEFT_SHOULDER;
        if (cp_pressed(pad.rightShoulder)) buttons |= CP_BUTTON_RIGHT_SHOULDER;
        if (cp_pressed(pad.buttonA)) buttons |= CP_BUTTON_A;
        if (cp_pressed(pad.buttonB)) buttons |= CP_BUTTON_B;
        if (cp_pressed(pad.buttonX)) buttons |= CP_BUTTON_X;
        if (cp_pressed(pad.buttonY)) buttons |= CP_BUTTON_Y;
        state->buttons = buttons;
        state->left_trigger = pad.leftTrigger.value;
        state->right_trigger = pad.rightTrigger.value;
        state->left_x = pad.leftThumbstick.xAxis.value;
        state->left_y = pad.leftThumbstick.yAxis.value;
        state->right_x = pad.rightThumbstick.xAxis.value;
        state->right_y = pad.rightThumbstick.yAxis.value;
        return 1;
    }
}

int cp_accessibility_trusted(void) {
    return CGPreflightPostEventAccess() ? 1 : 0;
}

void cp_request_accessibility(void) {
    CGRequestPostEventAccess();
}

int cp_pointer_move(int dx, int dy, int drag_button) {
    CGEventRef current = CGEventCreate(NULL);
    if (current == NULL) return -1;
    CGPoint point = CGEventGetLocation(current);
    CFRelease(current);
    point.x += dx;
    point.y += dy;
    CGEventType type = kCGEventMouseMoved;
    CGMouseButton button = kCGMouseButtonLeft;
    if (drag_button == 1) type = kCGEventLeftMouseDragged;
    if (drag_button == 2) {
        type = kCGEventRightMouseDragged;
        button = kCGMouseButtonRight;
    }
    CGEventRef event = CGEventCreateMouseEvent(NULL, type, point, button);
    if (event == NULL) return -1;
    CGEventPost(kCGHIDEventTap, event);
    CFRelease(event);
    return 0;
}

int cp_mouse_button(int button, int down) {
    CGEventRef current = CGEventCreate(NULL);
    if (current == NULL) return -1;
    CGPoint point = CGEventGetLocation(current);
    CFRelease(current);
    CGMouseButton mouse_button = button == 2 ? kCGMouseButtonRight : kCGMouseButtonLeft;
    CGEventType type;
    if (button == 2) type = down ? kCGEventRightMouseDown : kCGEventRightMouseUp;
    else type = down ? kCGEventLeftMouseDown : kCGEventLeftMouseUp;
    CGEventRef event = CGEventCreateMouseEvent(NULL, type, point, mouse_button);
    if (event == NULL) return -1;
    CGEventPost(kCGHIDEventTap, event);
    CFRelease(event);
    return 0;
}

int cp_scroll(int lines) {
    CGEventRef event = CGEventCreateScrollWheelEvent(NULL, kCGScrollEventUnitLine, 1, lines);
    if (event == NULL) return -1;
    CGEventPost(kCGHIDEventTap, event);
    CFRelease(event);
    return 0;
}

int cp_scroll_smooth(double pixels, int phase) {
    int32_t rounded = (int32_t)llround(pixels);
    CGEventRef event = CGEventCreateScrollWheelEvent(NULL, kCGScrollEventUnitPixel, 1, rounded);
    if (event == NULL) return -1;
    CGEventSetIntegerValueField(event, kCGScrollWheelEventIsContinuous, 1);
    CGEventSetDoubleValueField(event, kCGScrollWheelEventFixedPtDeltaAxis1, pixels);
    CGEventSetIntegerValueField(event, kCGScrollWheelEventPointDeltaAxis1, rounded);
    CGEventSetIntegerValueField(event, kCGScrollWheelEventScrollPhase, phase);
    CGEventPost(kCGHIDEventTap, event);
    CFRelease(event);
    return 0;
}

int cp_key_event(uint16_t keycode, int down, uint64_t flags) {
    CGEventRef event = CGEventCreateKeyboardEvent(NULL, (CGKeyCode)keycode, down ? true : false);
    if (event == NULL) return -1;
    CGEventSetFlags(event, (CGEventFlags)flags);
    CGEventPost(kCGHIDEventTap, event);
    CFRelease(event);
    return 0;
}

int cp_media_key(int key_type) {
    @autoreleasepool {
        for (int down = 1; down >= 0; down--) {
            int flags = down ? 0xA00 : 0xB00;
            NSEvent *event = [NSEvent otherEventWithType:NSEventTypeSystemDefined
                location:NSZeroPoint modifierFlags:flags timestamp:0
                windowNumber:0 context:nil subtype:8
                data1:(key_type << 16) | flags data2:-1];
            if (event == nil || event.CGEvent == NULL) return -1;
            CGEventPost(kCGHIDEventTap, event.CGEvent);
        }
        return 0;
    }
}

int cp_frontmost_executable(char *buffer, int length) {
    if (buffer == NULL || length <= 1) return -1;
    @autoreleasepool {
        NSRunningApplication *application = NSWorkspace.sharedWorkspace.frontmostApplication;
        NSString *path = application.executableURL.path;
        if (path == nil) return -1;
        const char *utf8 = path.UTF8String;
        if (utf8 == NULL) return -1;
        size_t size = strlen(utf8);
        if (size >= (size_t)length) size = (size_t)length - 1;
        memcpy(buffer, utf8, size);
        buffer[size] = '\0';
        return (int)size;
    }
}
