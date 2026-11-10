#include <stdint.h>

void *lm_pip_create(uintptr_t window, uintptr_t video_view, uintptr_t callback, int paused);
void lm_pip_close(void *picture);
void lm_pip_pause(void *picture, int paused);
int lm_pip_enqueue(void *picture, void *pixel_buffer);

void *lm_pip_gl_create(int width, int height, uint32_t *fbo);
void lm_pip_gl_close(void *target);
int lm_pip_gl_enqueue(void *picture, void *target);
