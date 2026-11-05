#ifndef LIVE_MYGO_VT_DARWIN_H
#define LIVE_MYGO_VT_DARWIN_H

#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

typedef struct VTCore VTCore;

/* Demuxing. */
VTCore *vt_core_open(const char *url, const char *user_agent, const char *referer,
                     const char *headers, const char *cafile,
                     char *err, int errlen);
void vt_core_stop(VTCore *core);
void vt_core_close(VTCore *core);
int vt_core_video_codec(VTCore *core);
int vt_codec_h264(void);
int vt_codec_hevc(void);
int vt_codec_aac(void);
int vt_core_audio_codec(VTCore *core);
int vt_core_audio_sample_rate(VTCore *core);
int vt_core_audio_channels(VTCore *core);
const uint8_t *vt_core_audio_extradata(VTCore *core, int *size);
const uint8_t *vt_core_extradata(VTCore *core, int *size);
int vt_core_width(VTCore *core);
int vt_core_height(VTCore *core);
double vt_core_fps(VTCore *core);

/* 0 = packet, 1 = end of stream, -1 = error. */
int vt_core_read(VTCore *core, uint8_t **data, int *size,
                 double *pts, double *dts, int *keyframe, int *kind,
                 char *err, int errlen);
void vt_core_free_packet(uint8_t *data);
/* Copy compressed packets to a short Matroska clip, without another connection. */
int vt_core_record_start(VTCore *core, const char *path, char *err, int errlen);
int vt_core_record_stop(VTCore *core, char *err, int errlen);

/* VideoToolbox decoding. */
int vt_core_start_decoder(VTCore *core, char *err, int errlen);
int vt_core_decode(VTCore *core, const uint8_t *data, int size,
                   double pts, double dts, int keyframe,
                   char *err, int errlen);
/* 1 = a frame was taken, 0 = none due. */
int vt_core_take_oldest(VTCore *core, void **frame, double *pts);
int vt_core_take_newest(VTCore *core, void **frame, double *pts);
int vt_core_queue_count(VTCore *core);
void vt_core_flush_frames(VTCore *core);
void vt_core_release_frame(void *frame);
int vt_core_dropped(VTCore *core);
int vt_core_decoded(VTCore *core);
void vt_core_peek_pts(VTCore *core, double *oldest, double *newest, int *count);
int vt_core_error(VTCore *core, char *err, int errlen);

/* AAC playback through AVSampleBufferAudioRenderer. */
void *vt_audio_create(int sample_rate, int channels, const uint8_t *extradata,
                      int size, char *err, int errlen);
void vt_audio_reset(void *audio);
void vt_audio_start(void *audio, double pts);
int vt_audio_ready(void *audio);
int vt_audio_enqueue(void *audio, const uint8_t *data, int size, double pts,
                     double duration, char *err, int errlen);
double vt_audio_clock(void *audio);
void vt_audio_set_rate(void *audio, double rate);
void vt_audio_set_volume(void *audio, double volume);
void vt_audio_set_muted(void *audio, int muted);
void vt_audio_close(void *audio);

/* CAMetalLayer surface. */
void *vt_view_create(uintptr_t ns_window);
void vt_view_close(void *view);
int vt_view_set_bounds(void *view, double x, double y, double w, double h,
                       double window_height, int *pixel_w, int *pixel_h, int *changed);
void vt_view_show(void *view, int show, int *visible);
void vt_view_set_corners(void *view, double radius, int right);
void vt_view_set_fit(void *view, int keep_aspect, int fill);
void vt_view_set_buttons(void *view, int visible);
int vt_view_present(void *view, void *frame);
int vt_view_save_png(void *view, const char *path, char *err, int errlen);
uint64_t vt_view_rendered(void *view);
const char *vt_view_error(void *view);

#ifdef __cplusplus
}
#endif

#endif
