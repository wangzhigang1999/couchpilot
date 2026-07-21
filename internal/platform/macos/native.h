#ifndef COUCHPILOT_MACOS_NATIVE_H
#define COUCHPILOT_MACOS_NATIVE_H

#include <stdint.h>

enum cp_gamepad_button {
    CP_BUTTON_DPAD_UP = 1u << 0,
    CP_BUTTON_DPAD_DOWN = 1u << 1,
    CP_BUTTON_DPAD_LEFT = 1u << 2,
    CP_BUTTON_DPAD_RIGHT = 1u << 3,
    CP_BUTTON_START = 1u << 4,
    CP_BUTTON_BACK = 1u << 5,
    CP_BUTTON_LEFT_THUMB = 1u << 6,
    CP_BUTTON_RIGHT_THUMB = 1u << 7,
    CP_BUTTON_LEFT_SHOULDER = 1u << 8,
    CP_BUTTON_RIGHT_SHOULDER = 1u << 9,
    CP_BUTTON_A = 1u << 10,
    CP_BUTTON_B = 1u << 11,
    CP_BUTTON_X = 1u << 12,
    CP_BUTTON_Y = 1u << 13
};

enum cp_gamepad_backend {
    CP_GAMEPAD_BACKEND_GAMECONTROLLER = 1,
    CP_GAMEPAD_BACKEND_HID = 2
};

typedef struct {
    uint16_t buttons;
    float left_trigger;
    float right_trigger;
    float left_x;
    float left_y;
    float right_x;
    float right_y;
} cp_gamepad_state;

int cp_gamepad_initialize(void);
int cp_gamepad_count(void);
int cp_gamepad_device_at(int index, int *backend, uint64_t *token);
int cp_gamepad_read(int backend, uint64_t token, cp_gamepad_state *state);
int cp_gamepad_diagnostic(int backend, uint64_t token, char *buffer, int length);

int cp_accessibility_trusted(void);
void cp_request_accessibility(void);
int cp_pointer_move(int dx, int dy, int drag_button);
int cp_mouse_button(int button, int down);
int cp_scroll(int lines);
int cp_scroll_smooth(double pixels, int phase);
int cp_key_event(uint16_t keycode, int down, uint64_t flags);
int cp_media_key(int key_type);
int cp_frontmost_executable(char *buffer, int length);

#endif
