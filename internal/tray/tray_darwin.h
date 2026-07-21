#ifndef COUCHPILOT_TRAY_DARWIN_H
#define COUCHPILOT_TRAY_DARWIN_H

int cp_tray_start(const unsigned char *icon, int icon_length);
int cp_tray_run_main_loop(void);
void cp_tray_stop(void);
void cp_tray_dispose(void);

#endif
