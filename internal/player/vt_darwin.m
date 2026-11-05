//go:build darwin && cgo

#import "vt_darwin.h"

#import <AppKit/AppKit.h>
#import <AudioToolbox/AudioToolbox.h>
#import <AVFoundation/AVFoundation.h>
#import <CoreImage/CoreImage.h>
#import <CoreMedia/CoreMedia.h>
#import <CoreVideo/CoreVideo.h>
#import <Foundation/Foundation.h>
#import <ImageIO/ImageIO.h>
#import <Metal/Metal.h>
#import <QuartzCore/QuartzCore.h>
#import <VideoToolbox/VideoToolbox.h>

#include <libavcodec/avcodec.h>
#include <libavformat/avformat.h>
#include <libavutil/avutil.h>
#include <libavutil/time.h>

#include <math.h>
#include <pthread.h>
#include <stdatomic.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

#define VT_FRAME_QUEUE 12

typedef struct {
    CVPixelBufferRef pb;
    double pts;
} VTFrame;

struct VTCore {
    AVFormatContext *fmt;
    AVPacket *packet;
    int video_index;
    AVRational time_base;
    double fps;
    int width, height;
    int codec_id;
    uint8_t *extradata;
    int extradata_size;
    double last_packet_pts;
    double last_queued_pts;

    int audio_index;
    AVRational audio_time_base;
    int audio_codec_id;
    int audio_sample_rate;
    int audio_channels;
    uint8_t *audio_extradata;
    int audio_extradata_size;
    double last_audio_pts;

    atomic_int stop;
    pthread_mutex_t lock;
    pthread_mutex_t error_lock;
    pthread_mutex_t record_lock;
    AVFormatContext *record_fmt;
    int record_video_index, record_audio_index;
    int record_started, record_error, record_packets;
    int64_t record_base_us;
    VTFrame frames[VT_FRAME_QUEUE];
    int frame_count;
    int dropped;
    int decoded;
    int in_flight;
    char error[512];

    CMVideoFormatDescriptionRef format;
    VTDecompressionSessionRef session;
};

static void vt_set_error(VTCore *core, const char *message) {
    if (!core || !message) return;
    pthread_mutex_lock(&core->error_lock);
    snprintf(core->error, sizeof(core->error), "%s", message);
    pthread_mutex_unlock(&core->error_lock);
}

static void vt_set_status_error(VTCore *core, const char *prefix, OSStatus status) {
    char message[512];
    snprintf(message, sizeof(message), "%s（%d）", prefix, (int)status);
    vt_set_error(core, message);
}

static void vt_set_av_error(VTCore *core, const char *prefix, int code) {
    char detail[256];
    av_strerror(code, detail, sizeof(detail));
    char message[512];
    snprintf(message, sizeof(message), "%s：%s", prefix, detail);
    vt_set_error(core, message);
}

static int vt_interrupt(void *opaque) {
    VTCore *core = (VTCore *)opaque;
    return core ? atomic_load(&core->stop) : 1;
}

static pthread_once_t vt_network_once = PTHREAD_ONCE_INIT;
static void vt_network_init(void) { avformat_network_init(); }

static void vt_flush_locked(VTCore *core) {
    for (int i = 0; i < core->frame_count; i++) {
        if (core->frames[i].pb) CFRelease(core->frames[i].pb);
        core->frames[i].pb = NULL;
    }
    core->frame_count = 0;
}

static void vt_record_packet(VTCore *core, const AVPacket *packet) {
    pthread_mutex_lock(&core->record_lock);
    AVFormatContext *output = core->record_fmt;
    if (!output || core->record_error) goto done;
    if (!core->record_started) {
        if (packet->stream_index != core->video_index || !(packet->flags & AV_PKT_FLAG_KEY)) goto done;
        int64_t timestamp = packet->dts != AV_NOPTS_VALUE ? packet->dts : packet->pts;
        if (timestamp == AV_NOPTS_VALUE) { core->record_error = AVERROR_INVALIDDATA; goto done; }
        core->record_base_us = av_rescale_q(timestamp, core->time_base, AV_TIME_BASE_Q);
        core->record_started = 1;
    }
    int index = packet->stream_index == core->video_index ? core->record_video_index : core->record_audio_index;
    if (index < 0) goto done;
    AVPacket *copy = av_packet_clone(packet);
    if (!copy) { core->record_error = AVERROR(ENOMEM); goto done; }
    AVRational source_time = core->fmt->streams[packet->stream_index]->time_base;
    int64_t base = av_rescale_q(core->record_base_us, AV_TIME_BASE_Q, source_time);
    if (copy->pts != AV_NOPTS_VALUE) copy->pts -= base;
    if (copy->dts != AV_NOPTS_VALUE) copy->dts -= base;
    av_packet_rescale_ts(copy, source_time, output->streams[index]->time_base);
    copy->stream_index = index;
    copy->pos = -1;
    int result = av_interleaved_write_frame(output, copy);
    av_packet_free(&copy);
    if (result < 0) core->record_error = result;
    else core->record_packets++;
done:
    pthread_mutex_unlock(&core->record_lock);
}

int vt_core_record_stop(VTCore *core, char *err, int errlen) {
    if (err && errlen > 0) err[0] = '\0';
    if (!core) return 0;
    pthread_mutex_lock(&core->record_lock);
    AVFormatContext *output = core->record_fmt;
    int result = 0;
    if (output) {
        result = av_write_trailer(output);
        if (core->record_error) result = core->record_error;
        else if (!core->record_packets) result = AVERROR_INVALIDDATA;
        if (output->pb) {
            int closed = avio_closep(&output->pb);
            if (result >= 0 && closed < 0) result = closed;
        }
        avformat_free_context(output);
        core->record_fmt = NULL;
    }
    pthread_mutex_unlock(&core->record_lock);
    if (result < 0 && err) {
        char detail[128]; av_strerror(result, detail, sizeof(detail));
        snprintf(err, errlen, "片段保存失败：%s", detail);
    }
    return result < 0 ? -1 : 0;
}

int vt_core_record_start(VTCore *core, const char *path, char *err, int errlen) {
    if (err && errlen > 0) err[0] = '\0';
    if (!core || !core->fmt || !path || !*path) {
        if (err) snprintf(err, errlen, "没有正在播放的直播");
        return -1;
    }
    vt_core_record_stop(core, NULL, 0);
    pthread_mutex_lock(&core->record_lock);
    AVFormatContext *output = NULL;
    int result = avformat_alloc_output_context2(&output, NULL, "matroska", path);
    if (result < 0 || !output) { result = AVERROR(ENOMEM); goto failed; }
    core->record_video_index = core->record_audio_index = -1;
    for (unsigned int i = 0; i < core->fmt->nb_streams; i++) {
        if ((int)i != core->video_index && (int)i != core->audio_index) continue;
        AVStream *stream = avformat_new_stream(output, NULL);
        if (!stream) { result = AVERROR(ENOMEM); goto failed; }
        result = avcodec_parameters_copy(stream->codecpar, core->fmt->streams[i]->codecpar);
        if (result < 0) goto failed;
        stream->codecpar->codec_tag = 0;
        stream->time_base = core->fmt->streams[i]->time_base;
        if ((int)i == core->video_index) core->record_video_index = stream->index;
        else core->record_audio_index = stream->index;
    }
    output->avoid_negative_ts = AVFMT_AVOID_NEG_TS_MAKE_NON_NEGATIVE;
    result = avio_open(&output->pb, path, AVIO_FLAG_WRITE);
    if (result < 0) goto failed;
    result = avformat_write_header(output, NULL);
    if (result < 0) goto failed;
    core->record_started = core->record_error = core->record_packets = 0;
    core->record_fmt = output;
    pthread_mutex_unlock(&core->record_lock);
    return 0;
failed:
    if (output) {
        if (output->pb) avio_closep(&output->pb);
        avformat_free_context(output);
    }
    pthread_mutex_unlock(&core->record_lock);
    if (err) {
        char detail[128]; av_strerror(result, detail, sizeof(detail));
        snprintf(err, errlen, "无法开始录制：%s", detail);
    }
    return -1;
}

VTCore *vt_core_open(const char *url, const char *user_agent, const char *referer,
                     const char *headers, const char *cafile,
                     char *err, int errlen) {
    if (err && errlen > 0) err[0] = '\0';
    if (!url || !*url) {
        if (err) snprintf(err, errlen, "直播地址为空");
        return NULL;
    }
    pthread_once(&vt_network_once, vt_network_init);
    const char *debug_log = getenv("LIVE_MYGO_VT_FFMPEG_DEBUG");
    if (debug_log && debug_log[0] == '1') av_log_set_level(AV_LOG_DEBUG);

    VTCore *core = calloc(1, sizeof(VTCore));
    if (!core) {
        if (err) snprintf(err, errlen, "内存不足");
        return NULL;
    }
    atomic_init(&core->stop, 0);
    pthread_mutex_init(&core->lock, NULL);
    pthread_mutex_init(&core->error_lock, NULL);
    pthread_mutex_init(&core->record_lock, NULL);
    core->last_packet_pts = NAN;
    core->last_queued_pts = NAN;
    core->last_audio_pts = NAN;
    core->audio_index = -1;

    AVFormatContext *fmt = avformat_alloc_context();
    if (!fmt) {
        if (err) snprintf(err, errlen, "无法创建解封装上下文");
        vt_core_close(core);
        return NULL;
    }
    fmt->interrupt_callback.callback = vt_interrupt;
    fmt->interrupt_callback.opaque = core;

    AVDictionary *options = NULL;
    if (user_agent && *user_agent) av_dict_set(&options, "user_agent", user_agent, 0);
    if (referer && *referer) av_dict_set(&options, "referer", referer, 0);
    if (headers && *headers) av_dict_set(&options, "headers", headers, 0);
    av_dict_set(&options, "tls_verify", "1", 0);
    av_dict_set(&options, "rw_timeout", "10000000", 0);
    // FLV 断开后需要重新取流，不能按旧字节偏移拼接新 HTTP 响应。
    // 错误交给应用层的有界重连，重新建立解封装和解码会话。
    if (cafile && *cafile) av_dict_set(&options, "ca_file", cafile, 0);

    int ret = avformat_open_input(&fmt, url, NULL, &options);
    av_dict_free(&options);
    if (ret < 0) {
        vt_set_av_error(core, "打开直播流失败", ret);
        if (err) snprintf(err, errlen, "%s", core->error);
        if (fmt) avformat_close_input(&fmt);
        vt_core_close(core);
        return NULL;
    }
    core->fmt = fmt;

    ret = avformat_find_stream_info(fmt, NULL);
    if (ret < 0) {
        vt_set_av_error(core, "读取直播流信息失败", ret);
        if (err) snprintf(err, errlen, "%s", core->error);
        vt_core_close(core);
        return NULL;
    }

    int index = av_find_best_stream(fmt, AVMEDIA_TYPE_VIDEO, -1, -1, NULL, 0);
    if (index < 0) {
        if (err) snprintf(err, errlen, "直播流中没有视频轨道");
        vt_core_close(core);
        return NULL;
    }
    core->video_index = index;
    AVStream *stream = fmt->streams[index];
    AVCodecParameters *codec = stream->codecpar;
    core->codec_id = codec->codec_id;
    core->width = codec->width;
    core->height = codec->height;
    core->time_base = stream->time_base;
    AVRational rate = av_guess_frame_rate(fmt, stream, NULL);
    core->fps = rate.num > 0 && rate.den > 0 ? av_q2d(rate) : 0;
    if (codec->extradata && codec->extradata_size > 0) {
        core->extradata = malloc((size_t)codec->extradata_size);
        if (!core->extradata) {
            if (err) snprintf(err, errlen, "内存不足");
            vt_core_close(core);
            return NULL;
        }
        memcpy(core->extradata, codec->extradata, (size_t)codec->extradata_size);
        core->extradata_size = codec->extradata_size;
    }
    core->packet = av_packet_alloc();
    if (!core->packet) {
        if (err) snprintf(err, errlen, "无法创建解封装包");
        vt_core_close(core);
        return NULL;
    }

    int audio = av_find_best_stream(fmt, AVMEDIA_TYPE_AUDIO, -1, -1, NULL, 0);
    if (audio >= 0) {
        AVStream *audio_stream = fmt->streams[audio];
        AVCodecParameters *audio_codec = audio_stream->codecpar;
        core->audio_index = audio;
        core->audio_time_base = audio_stream->time_base;
        core->audio_codec_id = audio_codec->codec_id;
        core->audio_sample_rate = audio_codec->sample_rate;
        core->audio_channels = audio_codec->ch_layout.nb_channels;
        if (audio_codec->extradata && audio_codec->extradata_size > 0) {
            core->audio_extradata = malloc((size_t)audio_codec->extradata_size);
            if (core->audio_extradata) {
                memcpy(core->audio_extradata, audio_codec->extradata, (size_t)audio_codec->extradata_size);
                core->audio_extradata_size = audio_codec->extradata_size;
            }
        }
    }
    return core;
}

void vt_core_stop(VTCore *core) {
    if (core) atomic_store(&core->stop, 1);
}

int vt_core_video_codec(VTCore *core) { return core ? core->codec_id : 0; }
int vt_codec_h264(void) { return AV_CODEC_ID_H264; }
int vt_codec_hevc(void) { return AV_CODEC_ID_HEVC; }
int vt_codec_aac(void) { return AV_CODEC_ID_AAC; }
int vt_core_audio_codec(VTCore *core) { return core ? core->audio_codec_id : 0; }
int vt_core_audio_sample_rate(VTCore *core) { return core ? core->audio_sample_rate : 0; }
int vt_core_audio_channels(VTCore *core) { return core ? core->audio_channels : 0; }
const uint8_t *vt_core_audio_extradata(VTCore *core, int *size) {
    if (!core || !core->audio_extradata) return NULL;
    if (size) *size = core->audio_extradata_size;
    return core->audio_extradata;
}
const uint8_t *vt_core_extradata(VTCore *core, int *size) {
    if (!core || !core->extradata) return NULL;
    if (size) *size = core->extradata_size;
    return core->extradata;
}
int vt_core_width(VTCore *core) { return core ? core->width : 0; }
int vt_core_height(VTCore *core) { return core ? core->height : 0; }
double vt_core_fps(VTCore *core) { return core ? core->fps : 0; }

int vt_core_read(VTCore *core, uint8_t **data, int *size,
                 double *pts, double *dts, int *keyframe, int *kind,
                 char *err, int errlen) {
    if (err && errlen > 0) err[0] = '\0';
    if (!core || !core->fmt || !core->packet) {
        if (err) snprintf(err, errlen, "播放器未初始化");
        return -1;
    }
    for (;;) {
        if (atomic_load(&core->stop)) return 1;
        int ret = av_read_frame(core->fmt, core->packet);
        if (ret == AVERROR(EAGAIN)) {
            av_usleep(10000);
            continue;
        }
        if (ret == AVERROR_EOF) return 1;
        if (ret < 0) {
            if (atomic_load(&core->stop)) return 1;
            vt_set_av_error(core, "读取直播数据失败", ret);
            if (err) snprintf(err, errlen, "%s", core->error);
            return -1;
        }
        int stream = core->packet->stream_index;
        int is_video = stream == core->video_index;
        int is_audio = core->audio_index >= 0 && stream == core->audio_index;
        if (!is_video && !is_audio) {
            av_packet_unref(core->packet);
            continue;
        }
        if (core->packet->size <= 0) {
            av_packet_unref(core->packet);
            continue;
        }
        vt_record_packet(core, core->packet);
        uint8_t *copy = malloc((size_t)core->packet->size);
        if (!copy) {
            av_packet_unref(core->packet);
            if (err) snprintf(err, errlen, "内存不足");
            return -1;
        }
        memcpy(copy, core->packet->data, (size_t)core->packet->size);
        double presentation;
        double decode;
        if (is_video) {
            double scale = av_q2d(core->time_base);
            presentation = core->packet->pts != AV_NOPTS_VALUE ? (double)core->packet->pts * scale : NAN;
            decode = core->packet->dts != AV_NOPTS_VALUE ? (double)core->packet->dts * scale : NAN;
            if (!isfinite(presentation)) {
                presentation = isfinite(decode) ? decode : core->last_packet_pts + (core->fps > 0 ? 1.0 / core->fps : .033);
            }
            if (!isfinite(decode)) decode = presentation;
            core->last_packet_pts = presentation;
            *keyframe = (core->packet->flags & AV_PKT_FLAG_KEY) != 0;
            *kind = 0;
        } else {
            double scale = av_q2d(core->audio_time_base);
            presentation = core->packet->pts != AV_NOPTS_VALUE ? (double)core->packet->pts * scale : NAN;
            decode = core->packet->dts != AV_NOPTS_VALUE ? (double)core->packet->dts * scale : NAN;
            if (!isfinite(presentation)) {
                presentation = isfinite(decode) ? decode
                    : core->last_audio_pts + (core->audio_sample_rate > 0 ? 1024.0 / core->audio_sample_rate : .064);
            }
            core->last_audio_pts = presentation;
            *keyframe = 1;
            *kind = 1;
            decode = presentation;
        }
        *data = copy;
        *size = core->packet->size;
        *pts = presentation;
        *dts = decode;
        av_packet_unref(core->packet);
        return 0;
    }
}

void vt_core_free_packet(uint8_t *data) {
    free(data);
}

static int vt_make_format_description(VTCore *core) {
    const uint8_t *data = core->extradata;
    int size = core->extradata_size;
    if (!data || size < 7) {
        vt_set_error(core, "直播流缺少解码器配置");
        return -1;
    }
    const uint8_t *sets[64];
    size_t lengths[64];
    int count = 0;
    int nal_length = 4;
    OSStatus status = noErr;

    if (core->codec_id == AV_CODEC_ID_H264) {
        nal_length = (data[4] & 3) + 1;
        int sps_count = data[5] & 0x1f;
        int offset = 6;
        for (int i = 0; i < sps_count; i++) {
            if (offset + 2 > size) goto invalid;
            int length = (data[offset] << 8) | data[offset + 1];
            offset += 2;
            if (length <= 0 || offset + length > size || count >= 64) goto invalid;
            sets[count] = data + offset;
            lengths[count] = (size_t)length;
            count++;
            offset += length;
        }
        if (offset >= size) goto invalid;
        int pps_count = data[offset++];
        for (int i = 0; i < pps_count; i++) {
            if (offset + 2 > size) goto invalid;
            int length = (data[offset] << 8) | data[offset + 1];
            offset += 2;
            if (length <= 0 || offset + length > size || count >= 64) goto invalid;
            sets[count] = data + offset;
            lengths[count] = (size_t)length;
            count++;
            offset += length;
        }
        if (count == 0) goto invalid;
        status = CMVideoFormatDescriptionCreateFromH264ParameterSets(
            kCFAllocatorDefault, (size_t)count, sets, lengths, nal_length, &core->format);
    } else if (core->codec_id == AV_CODEC_ID_HEVC) {
        if (size < 23) goto invalid;
        int arrays = data[22];
        int offset = 23;
        for (int array = 0; array < arrays; array++) {
            if (offset + 3 > size) goto invalid;
            offset++;
            int nals = (data[offset] << 8) | data[offset + 1];
            offset += 2;
            for (int i = 0; i < nals; i++) {
                if (offset + 2 > size) goto invalid;
                int length = (data[offset] << 8) | data[offset + 1];
                offset += 2;
                if (length <= 0 || offset + length > size || count >= 64) goto invalid;
                sets[count] = data + offset;
                lengths[count] = (size_t)length;
                count++;
                offset += length;
            }
        }
        if (count == 0) goto invalid;
        status = CMVideoFormatDescriptionCreateFromHEVCParameterSets(
            kCFAllocatorDefault, (size_t)count, sets, lengths, nal_length, NULL, &core->format);
    } else {
        char message[128];
        snprintf(message, sizeof(message), "测试后端不支持编码 %d", core->codec_id);
        vt_set_error(core, message);
        return -1;
    }

    if (status != noErr || !core->format) {
        vt_set_status_error(core, "无法创建视频格式描述", status);
        return -1;
    }
    return 0;

invalid:
    vt_set_error(core, "视频解码器配置无效");
    return -1;
}

static void vt_decode_callback(void *refcon, void *source, OSStatus status,
                               VTDecodeInfoFlags flags, CVImageBufferRef image,
                               CMTime presentation, CMTime duration) {
    VTCore *core = (VTCore *)refcon;
    pthread_mutex_lock(&core->lock);
    core->in_flight--;
    if (status != noErr) {
        pthread_mutex_unlock(&core->lock);
        vt_set_status_error(core, "VideoToolbox 解码失败", status);
        return;
    }
    if (!image) {
        if (flags & kVTDecodeInfo_FrameDropped) {
            core->dropped++;
        }
        pthread_mutex_unlock(&core->lock);
        return;
    }
    CVPixelBufferRef pixel = (CVPixelBufferRef)image;
    CFRetain(pixel);
    double seconds = CMTimeGetSeconds(presentation);
    if (!isfinite(seconds)) {
        seconds = isfinite(core->last_queued_pts) ? core->last_queued_pts + (core->fps > 0 ? 1.0 / core->fps : .033) : 0;
    }
    core->last_queued_pts = seconds;
    if (core->frame_count == VT_FRAME_QUEUE) {
        int oldest = 0;
        for (int i = 1; i < core->frame_count; i++) {
            if (core->frames[i].pts < core->frames[oldest].pts) oldest = i;
        }
        CFRelease(core->frames[oldest].pb);
        memmove(&core->frames[oldest], &core->frames[oldest + 1],
                sizeof(VTFrame) * (size_t)(core->frame_count - oldest - 1));
        core->frame_count--;
        core->dropped++;
    }
    int at = core->frame_count;
    while (at > 0 && core->frames[at - 1].pts > seconds) {
        core->frames[at] = core->frames[at - 1];
        at--;
    }
    core->frames[at].pb = pixel;
    core->frames[at].pts = seconds;
    core->frame_count++;
    core->decoded++;
    pthread_mutex_unlock(&core->lock);
}

int vt_core_start_decoder(VTCore *core, char *err, int errlen) {
    if (err && errlen > 0) err[0] = '\0';
    if (!core) return -1;
    if (core->session) {
        VTDecompressionSessionWaitForAsynchronousFrames(core->session);
        VTDecompressionSessionInvalidate(core->session);
        CFRelease(core->session);
        core->session = NULL;
    }
    if (core->format) {
        CFRelease(core->format);
        core->format = NULL;
    }
    if (vt_make_format_description(core) != 0) {
        if (err) snprintf(err, errlen, "%s", core->error);
        return -1;
    }

    CFMutableDictionaryRef attributes = CFDictionaryCreateMutable(
        kCFAllocatorDefault, 0, &kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
    int pixel_format = kCVPixelFormatType_420YpCbCr8BiPlanarVideoRange;
    CFNumberRef number = CFNumberCreate(kCFAllocatorDefault, kCFNumberSInt32Type, &pixel_format);
    CFDictionarySetValue(attributes, kCVPixelBufferPixelFormatTypeKey, number);
    CFRelease(number);
    CFDictionarySetValue(attributes, kCVPixelBufferMetalCompatibilityKey, kCFBooleanTrue);
    CFDictionaryRef empty = CFDictionaryCreate(kCFAllocatorDefault, NULL, NULL, 0, NULL, NULL);
    CFDictionarySetValue(attributes, kCVPixelBufferIOSurfacePropertiesKey, empty);
    CFRelease(empty);

    VTDecompressionOutputCallbackRecord record = {vt_decode_callback, core};
    OSStatus status = VTDecompressionSessionCreate(
        kCFAllocatorDefault, core->format, (__bridge CFDictionaryRef)@{(__bridge NSString *)kVTVideoDecoderSpecification_RequireHardwareAcceleratedVideoDecoder: @YES}, attributes, &record, &core->session);
    CFRelease(attributes);
    if (status != noErr || !core->session) {
        vt_set_status_error(core, "无法创建 VideoToolbox 解码器", status);
        if (err) snprintf(err, errlen, "%s", core->error);
        return -1;
    }
    VTSessionSetProperty(core->session, kVTDecompressionPropertyKey_RealTime, kCFBooleanTrue);
    return 0;
}

int vt_core_decode(VTCore *core, const uint8_t *data, int size,
                   double pts, double dts, int keyframe,
                   char *err, int errlen) {
    if (err && errlen > 0) err[0] = '\0';
    if (!core || !core->session || !core->format || !data || size <= 0) {
        if (err) snprintf(err, errlen, "解码器未就绪");
        return -1;
    }
    CMBlockBufferRef block = NULL;
    OSStatus status = CMBlockBufferCreateWithMemoryBlock(
        kCFAllocatorDefault, NULL, (size_t)size, kCFAllocatorDefault, NULL,
        0, (size_t)size, kCMBlockBufferAssureMemoryNowFlag, &block);
    if (status != noErr || !block) {
        vt_set_status_error(core, "无法创建视频数据块", status);
        if (err) snprintf(err, errlen, "%s", core->error);
        return -1;
    }
    char *destination = NULL;
    status = CMBlockBufferGetDataPointer(block, 0, NULL, NULL, &destination);
    if (status != noErr || !destination) {
        CFRelease(block);
        vt_set_status_error(core, "无法写入视频数据块", status);
        if (err) snprintf(err, errlen, "%s", core->error);
        return -1;
    }
    memcpy(destination, data, (size_t)size);

    double frame_duration = core->fps > 0 ? 1.0 / core->fps : 1.0 / 30;
    CMSampleTimingInfo timing;
    timing.duration = CMTimeMakeWithSeconds(frame_duration, 90000);
    timing.presentationTimeStamp = CMTimeMakeWithSeconds(pts, 90000);
    timing.decodeTimeStamp = CMTimeMakeWithSeconds(dts, 90000);
    size_t sample_size = (size_t)size;
    CMSampleBufferRef sample = NULL;
    status = CMSampleBufferCreateReady(
        kCFAllocatorDefault, block, core->format, 1, 1, &timing, 1, &sample_size, &sample);
    CFRelease(block);
    if (status != noErr || !sample) {
        vt_set_status_error(core, "无法创建视频样本", status);
        if (err) snprintf(err, errlen, "%s", core->error);
        return -1;
    }

    CFArrayRef attachments = CMSampleBufferGetSampleAttachmentsArray(sample, true);
    if (attachments && CFArrayGetCount(attachments) > 0 && !keyframe) {
        CFMutableDictionaryRef dictionary = (CFMutableDictionaryRef)CFArrayGetValueAtIndex(attachments, 0);
        if (dictionary) CFDictionarySetValue(dictionary, kCMSampleAttachmentKey_NotSync, kCFBooleanTrue);
    }

    VTDecodeInfoFlags flags = 0;
    pthread_mutex_lock(&core->lock);
    core->in_flight++;
    pthread_mutex_unlock(&core->lock);
    status = VTDecompressionSessionDecodeFrame(
        core->session, sample,
        kVTDecodeFrame_EnableAsynchronousDecompression | kVTDecodeFrame_EnableTemporalProcessing,
        NULL, &flags);
    CFRelease(sample);
    if (status != noErr) {
        pthread_mutex_lock(&core->lock);
        core->in_flight--;
        pthread_mutex_unlock(&core->lock);
        vt_set_status_error(core, "提交视频样本失败", status);
        if (err) snprintf(err, errlen, "%s", core->error);
        return -1;
    }
    return 0;
}

void vt_core_flush_frames(VTCore *core) {
    if (!core) return;
    pthread_mutex_lock(&core->lock);
    vt_flush_locked(core);
    pthread_mutex_unlock(&core->lock);
}

int vt_core_take_oldest(VTCore *core, void **frame, double *pts) {
    if (!core || !frame) return 0;
    *frame = NULL;
    pthread_mutex_lock(&core->lock);
    if (core->frame_count == 0) {
        pthread_mutex_unlock(&core->lock);
        return 0;
    }
    CVPixelBufferRef out = core->frames[0].pb;
    if (pts) *pts = core->frames[0].pts;
    memmove(&core->frames[0], &core->frames[1],
            sizeof(VTFrame) * (size_t)(core->frame_count - 1));
    core->frame_count--;
    pthread_mutex_unlock(&core->lock);
    *frame = (void *)out;
    return 1;
}

int vt_core_take_newest(VTCore *core, void **frame, double *pts) {
    if (!core || !frame) return 0;
    *frame = NULL;
    pthread_mutex_lock(&core->lock);
    if (core->frame_count == 0) {
        pthread_mutex_unlock(&core->lock);
        return 0;
    }
    int chosen = core->frame_count - 1;
    for (int i = 0; i < chosen; i++) {
        CFRelease(core->frames[i].pb);
        core->dropped++;
    }
    CVPixelBufferRef out = core->frames[chosen].pb;
    if (pts) *pts = core->frames[chosen].pts;
    core->frame_count = 0;
    pthread_mutex_unlock(&core->lock);
    *frame = (void *)out;
    return 1;
}

int vt_core_queue_count(VTCore *core) {
    if (!core) return 0;
    pthread_mutex_lock(&core->lock);
    int value = core->frame_count;
    pthread_mutex_unlock(&core->lock);
    return value;
}

int vt_core_pending_count(VTCore *core) {
    if (!core) return 0;
    pthread_mutex_lock(&core->lock);
    int count = core->frame_count + core->in_flight;
    pthread_mutex_unlock(&core->lock);
    return count;
}

void vt_core_release_frame(void *frame) {
    if (frame) CFRelease((CVPixelBufferRef)frame);
}

int vt_core_dropped(VTCore *core) {
    if (!core) return 0;
    pthread_mutex_lock(&core->lock);
    int value = core->dropped;
    pthread_mutex_unlock(&core->lock);
    return value;
}

int vt_core_decoded(VTCore *core) {
    if (!core) return 0;
    pthread_mutex_lock(&core->lock);
    int value = core->decoded;
    pthread_mutex_unlock(&core->lock);
    return value;
}

void vt_core_peek_pts(VTCore *core, double *oldest, double *newest, int *count) {
    if (!core) return;
    pthread_mutex_lock(&core->lock);
    if (oldest) *oldest = core->frame_count > 0 ? core->frames[0].pts : NAN;
    if (newest) *newest = core->frame_count > 0 ? core->frames[core->frame_count - 1].pts : NAN;
    if (count) *count = core->frame_count;
    pthread_mutex_unlock(&core->lock);
}

int vt_core_error(VTCore *core, char *err, int errlen) {
    if (!core || !err || errlen <= 0) return 0;
    pthread_mutex_lock(&core->error_lock);
    int has = core->error[0] != '\0';
    if (has) snprintf(err, errlen, "%s", core->error);
    pthread_mutex_unlock(&core->error_lock);
    return has;
}

void vt_core_close(VTCore *core) {
    if (!core) return;
    atomic_store(&core->stop, 1);
    if (core->session) {
        VTDecompressionSessionWaitForAsynchronousFrames(core->session);
        VTDecompressionSessionInvalidate(core->session);
        CFRelease(core->session);
        core->session = NULL;
    }
    if (core->format) {
        CFRelease(core->format);
        core->format = NULL;
    }
    pthread_mutex_lock(&core->lock);
    vt_flush_locked(core);
    pthread_mutex_unlock(&core->lock);
    vt_core_record_stop(core, NULL, 0);
    if (core->packet) av_packet_free(&core->packet);
    if (core->fmt) avformat_close_input(&core->fmt);
    free(core->extradata);
    free(core->audio_extradata);
    pthread_mutex_destroy(&core->lock);
    pthread_mutex_destroy(&core->error_lock);
    pthread_mutex_destroy(&core->record_lock);
    free(core);
}

/* ------------------------------------------------------------------ */
/* AAC audio                                                          */
/* ------------------------------------------------------------------ */

@interface VTAudio : NSObject
@property(nonatomic, strong) AVSampleBufferRenderSynchronizer *synchronizer;
@property(nonatomic, strong) AVSampleBufferAudioRenderer *renderer;
@property(nonatomic, assign) CMFormatDescriptionRef format;
@property(atomic, assign) BOOL started;
@property(atomic, assign) double queuedEnd;
@property(nonatomic, assign) double originPTS;
@property(nonatomic, assign) int64_t queuedFrames;
@property(nonatomic, assign) int sampleRate;
@end

@implementation VTAudio
- (void)dealloc {
    if (_format) CFRelease(_format);
}
@end

void *vt_audio_create(int sample_rate, int channels, const uint8_t *extradata,
                      int size, char *err, int errlen) {
    @autoreleasepool {
    if (err && errlen > 0) err[0] = '\0';
    if (sample_rate <= 0 || channels <= 0) {
        if (err) snprintf(err, errlen, "音频参数无效");
        return NULL;
    }
    AudioStreamBasicDescription asbd;
    memset(&asbd, 0, sizeof(asbd));
    asbd.mSampleRate = sample_rate;
    asbd.mFormatID = kAudioFormatMPEG4AAC;
    asbd.mFormatFlags = 0;
    asbd.mFramesPerPacket = 1024;
    asbd.mChannelsPerFrame = (UInt32)channels;
    CMFormatDescriptionRef format = NULL;
    OSStatus status = CMAudioFormatDescriptionCreate(
        kCFAllocatorDefault, &asbd, 0, NULL,
        size > 0 ? (size_t)size : 0, size > 0 ? extradata : NULL, NULL, &format);
    if (status != noErr || !format) {
        if (err) snprintf(err, errlen, "无法创建音频格式描述（%d）", (int)status);
        return NULL;
    }
    VTAudio *audio = [[VTAudio alloc] init];
    audio.queuedEnd = NAN;
    audio.originPTS = NAN;
    audio.sampleRate = sample_rate;
    audio.format = format;
    audio.synchronizer = [[AVSampleBufferRenderSynchronizer alloc] init];
    audio.synchronizer.delaysRateChangeUntilHasSufficientMediaData = NO;
    audio.renderer = [[AVSampleBufferAudioRenderer alloc] init];
    [audio.synchronizer addRenderer:audio.renderer];
    return (__bridge_retained void *)audio;
    }
}

void vt_audio_start(void *handle, double pts) {
    @autoreleasepool {
    if (!handle) return;
    VTAudio *audio = (__bridge VTAudio *)handle;
    if (audio.started) return;
    // 给系统音频输出一个固定的短预填窗口，不等待 renderer 的大块文件预缓冲。
    CMTime host = CMTimeAdd(CMClockGetTime(CMClockGetHostTimeClock()), CMTimeMake(1, 10));
    [audio.synchronizer setRate:1.0 time:CMTimeMakeWithSeconds(fmax(0, pts), 90000) atHostTime:host];
    audio.started = YES;
    }
}

int vt_audio_ready(void *handle) {
    if (!handle) return 0;
    VTAudio *audio = (__bridge VTAudio *)handle;
    return audio.renderer.readyForMoreMediaData || audio.renderer.status == AVQueuedSampleBufferRenderingStatusFailed ? 1 : 0;
}

int vt_audio_enqueue(void *handle, const uint8_t *data, int size, double pts,
                     double duration, char *err, int errlen) {
    @autoreleasepool {
    if (err && errlen > 0) err[0] = '\0';
    if (!handle || !data || size <= 0) return -1;
    VTAudio *audio = (__bridge VTAudio *)handle;
    CMBlockBufferRef block = NULL;
    OSStatus status = CMBlockBufferCreateWithMemoryBlock(
        kCFAllocatorDefault, NULL, (size_t)size, kCFAllocatorDefault, NULL,
        0, (size_t)size, kCMBlockBufferAssureMemoryNowFlag, &block);
    if (status != noErr || !block) {
        if (err) snprintf(err, errlen, "无法创建音频数据块（%d）", (int)status);
        return -1;
    }
    char *destination = NULL;
    status = CMBlockBufferGetDataPointer(block, 0, NULL, NULL, &destination);
    if (status != noErr || !destination) {
        CFRelease(block);
        if (err) snprintf(err, errlen, "无法写入音频数据块（%d）", (int)status);
        return -1;
    }
    memcpy(destination, data, (size_t)size);

    CMSampleTimingInfo timing;
    // FLV 的毫秒时间戳把 48 kHz AAC 的 21⅓ ms 交替量化成 21 / 22 ms。
    // 按每包 1024 个样本连续排布，避免压缩音频缓冲相互重叠或出现小空洞。
    double expected = audio.originPTS + (double)audio.queuedFrames / audio.sampleRate;
    if (!isfinite(expected) || fabs(pts - expected) > duration / 2) {
        audio.originPTS = pts;
        audio.queuedFrames = 0;
    }
    timing.duration = CMTimeMake(1024, audio.sampleRate);
    timing.presentationTimeStamp = CMTimeAdd(CMTimeMakeWithSeconds(audio.originPTS, audio.sampleRate), CMTimeMake(audio.queuedFrames, audio.sampleRate));
    timing.decodeTimeStamp = kCMTimeInvalid;
    size_t sample_size = (size_t)size;
    CMSampleBufferRef sample = NULL;
    status = CMSampleBufferCreateReady(
        kCFAllocatorDefault, block, audio.format, 1, 1, &timing, 1, &sample_size, &sample);
    CFRelease(block);
    if (status != noErr || !sample) {
        if (err) snprintf(err, errlen, "无法创建音频样本（%d）", (int)status);
        return -1;
    }
    [audio.renderer enqueueSampleBuffer:sample];
    CFRelease(sample);
    if (audio.renderer.status == AVQueuedSampleBufferRenderingStatusFailed) {
        if (err) snprintf(err, errlen, "%s", audio.renderer.error.localizedDescription.UTF8String ?: "音频渲染失败");
        return -1;
    }
    audio.queuedFrames += 1024;
    audio.queuedEnd = CMTimeGetSeconds(CMTimeAdd(timing.presentationTimeStamp, timing.duration));
    return 0;
    }
}

double vt_audio_clock(void *handle) {
    @autoreleasepool {
    if (!handle) return NAN;
    VTAudio *audio = (__bridge VTAudio *)handle;
    if (!audio.started) return NAN;
    return CMTimeGetSeconds([audio.synchronizer currentTime]);
    }
}

void vt_audio_timing(void *handle, double *clock, double *buffered, double *rate) {
    @autoreleasepool {
        if (clock) *clock = NAN;
        if (buffered) *buffered = 0;
        if (rate) *rate = 0;
        if (!handle) return;
        VTAudio *audio = (__bridge VTAudio *)handle;
        if (!audio.started) return;
        double now = CMTimeGetSeconds([audio.synchronizer currentTime]);
        if (clock) *clock = now;
        if (buffered && isfinite(audio.queuedEnd) && isfinite(now)) *buffered = fmax(0, audio.queuedEnd - now);
        if (rate) *rate = CMTimebaseGetEffectiveRate(audio.synchronizer.timebase);
    }
}

void vt_audio_set_rate(void *handle, double rate) {
    @autoreleasepool {
    if (!handle) return;
    VTAudio *audio = (__bridge VTAudio *)handle;
    [audio.synchronizer setRate:(float)rate time:kCMTimeInvalid];
    }
}

void vt_audio_set_volume(void *handle, double volume) {
    @autoreleasepool {
    if (!handle) return;
    VTAudio *audio = (__bridge VTAudio *)handle;
    audio.renderer.volume = (float)volume;
    }
}

void vt_audio_set_muted(void *handle, int muted) {
    @autoreleasepool {
    if (!handle) return;
    VTAudio *audio = (__bridge VTAudio *)handle;
    audio.renderer.muted = muted != 0;
    }
}

void vt_audio_close(void *handle) {
    @autoreleasepool {
    if (!handle) return;
    VTAudio *audio = (__bridge_transfer VTAudio *)handle;
    [audio.synchronizer setRate:0 time:kCMTimeInvalid];
    }
}

/* ------------------------------------------------------------------ */
/* CAMetalLayer surface                                                */
/* ------------------------------------------------------------------ */

@interface VTMetalView : NSView
@property(nonatomic, strong) CAMetalLayer *metalLayer;
@property(nonatomic, strong) id<MTLCommandQueue> queue;
@property(nonatomic, strong) CIContext *ciContext;
@property(nonatomic, assign) CGColorSpaceRef colorSpace;
@property(nonatomic, copy) NSString *lastError;
@property(nonatomic, assign) uint64_t rendered;
@property(atomic, assign) BOOL shown;
@property(atomic, assign) BOOL keepAspect;
@property(atomic, assign) BOOL fillVideo;
@property(nonatomic, strong) CIImage *captureImage;
@end

@implementation VTMetalView

- (instancetype)initWithFrame:(NSRect)frame {
    self = [super initWithFrame:frame];
    if (!self) return nil;
    self.keepAspect = YES;
    self.wantsLayer = YES;
    self.layer.masksToBounds = YES;
    _metalLayer = [CAMetalLayer layer];
    _metalLayer.frame = self.bounds;
    _metalLayer.masksToBounds = YES;
    [self.layer addSublayer:_metalLayer];
    id<MTLDevice> device = MTLCreateSystemDefaultDevice();
    if (!device) {
        self.lastError = @"当前设备没有 Metal";
        return self;
    }
    _metalLayer.device = device;
    _metalLayer.pixelFormat = MTLPixelFormatBGRA8Unorm;
    _metalLayer.framebufferOnly = NO;
    _metalLayer.opaque = YES;
    _metalLayer.backgroundColor = CGColorGetConstantColor(kCGColorBlack);
    _metalLayer.presentsWithTransaction = NO;
    _metalLayer.displaySyncEnabled = YES;
    self.queue = [device newCommandQueue];
    if (!self.queue) {
        self.lastError = @"无法创建 Metal 命令队列";
        return self;
    }
    self.colorSpace = CGColorSpaceCreateWithName(kCGColorSpaceSRGB);
    _metalLayer.colorspace = self.colorSpace;
    self.ciContext = [CIContext contextWithMTLDevice:device
                                             options:@{kCIContextUseSoftwareRenderer: @NO}];
    if (!self.ciContext) {
        self.lastError = @"无法创建 CoreImage 上下文";
    }
    return self;
}

- (void)dealloc {
    if (_colorSpace) CGColorSpaceRelease(_colorSpace);
}

- (void)viewDidMoveToWindow {
    [super viewDidMoveToWindow];
    [self updateDrawable];
}

- (void)viewDidChangeBackingProperties {
    [super viewDidChangeBackingProperties];
    [self updateDrawable];
}

- (void)layout {
    [super layout];
    [self updateDrawable];
}

- (void)updateDrawable {
    if (!_metalLayer) return;
    CGFloat scale = self.window ? self.window.backingScaleFactor : 1.0;
    if (_metalLayer.contentsScale != scale) _metalLayer.contentsScale = scale;
    if (!NSEqualRects(_metalLayer.frame, self.bounds)) _metalLayer.frame = self.bounds;
    CGSize size = CGSizeMake(MAX(1, self.bounds.size.width * scale),
                             MAX(1, self.bounds.size.height * scale));
    if (!CGSizeEqualToSize(_metalLayer.drawableSize, size)) {
        _metalLayer.drawableSize = size;
    }
}

@end

void *vt_view_create(uintptr_t ns_window) {
    if (!ns_window) return NULL;
    NSWindow *window = (__bridge NSWindow *)(void *)ns_window;
    NSView *parent = window.contentView;
    if (!parent) return NULL;
    Class canvasClass = NSClassFromString(@"MyGoSurfaceView");
    NSView *canvas = nil;
    for (NSView *view in parent.subviews) {
        if ((canvasClass && [view isKindOfClass:canvasClass]) ||
            [NSStringFromClass(view.class) isEqualToString:@"MyGoSurfaceView"]) {
            canvas = view;
            break;
        }
    }
    if (!canvas) return NULL;
    VTMetalView *view = [[VTMetalView alloc] initWithFrame:NSMakeRect(0, 0, 640, 360)];
    if (view.lastError) return NULL;
    view.hidden = YES;
    [parent addSubview:view positioned:NSWindowBelow relativeTo:canvas];
    return (__bridge_retained void *)view;
}

void vt_view_close(void *handle) {
    if (!handle) return;
    VTMetalView *view = (__bridge_transfer VTMetalView *)handle;
    [view removeFromSuperview];
}

int vt_view_set_bounds(void *handle, double x, double y, double w, double h,
                       double window_height, int *pixel_w, int *pixel_h, int *changed) {
    if (!handle) return 0;
    VTMetalView *view = (__bridge VTMetalView *)handle;
    NSView *parent = view.superview;
    double top = y;
    if (parent && !parent.isFlipped) top = window_height - y - h;
    NSRect target = NSMakeRect(x, top, MAX(1, w), MAX(1, h));
    CGFloat scale = view.window ? view.window.backingScaleFactor : 1.0;
    CGSize expected = CGSizeMake(MAX(1, target.size.width * scale),
                                 MAX(1, target.size.height * scale));
    BOOL updated = NO;
    @synchronized(view) {
        CAMetalLayer *metal = view.metalLayer;
        if (!NSEqualRects(view.frame, target) ||
            !CGSizeEqualToSize(metal.drawableSize, expected) ||
            metal.contentsScale != scale) {
            [view setFrame:target];
            [view updateDrawable];
            updated = YES;
        }
        if (pixel_w) *pixel_w = (int)metal.drawableSize.width;
        if (pixel_h) *pixel_h = (int)metal.drawableSize.height;
    }
    if (changed) *changed = updated ? 1 : 0;
    return 1;
}

void vt_view_show(void *handle, int show, int *visible) {
    if (!handle) {
        if (visible) *visible = 0;
        return;
    }
    VTMetalView *view = (__bridge VTMetalView *)handle;
    NSWindow *window = view.window;
    BOOL onScreen = window && window.isVisible &&
                    (window.occlusionState & NSWindowOcclusionStateVisible) != 0;
    const char *force = getenv("LIVE_MYGO_VT_FORCE");
    if (force && force[0] == '1' && window) onScreen = YES;
    BOOL shown = show && onScreen;
    view.shown = shown;
    view.hidden = !shown;
    if (visible) *visible = shown ? 1 : 0;
}

void vt_view_set_corners(void *handle, double radius, int right) {
    if (!handle) return;
    VTMetalView *view = (__bridge VTMetalView *)handle;
    CAMetalLayer *metal = view.metalLayer;
    @synchronized(view) {
        metal.cornerRadius = MAX(0, radius);
        metal.cornerCurve = kCACornerCurveContinuous;
        CACornerMask mask = 0;
        if (radius > 0) {
            mask = kCALayerMinXMinYCorner | kCALayerMinXMaxYCorner;
            if (right) mask |= kCALayerMaxXMinYCorner | kCALayerMaxXMaxYCorner;
        }
        metal.maskedCorners = mask;
    }
}

void vt_view_set_fit(void *handle, int keep_aspect, int fill) {
    if (!handle) return;
    VTMetalView *view = (__bridge VTMetalView *)handle;
    view.keepAspect = keep_aspect != 0;
    view.fillVideo = fill != 0;
}

void vt_view_set_buttons(void *handle, int visible) {
    if (!handle) return;
    VTMetalView *view = (__bridge VTMetalView *)handle;
    NSWindow *window = view.window;
    if (!window) return;
    for (NSInteger kind = 0; kind < 3; kind++) {
        NSButton *button = [window standardWindowButton:(NSWindowButton)kind];
        if (button) button.hidden = !visible;
    }
}

int vt_view_present(void *handle, void *frame) {
    @autoreleasepool {
    if (!handle || !frame) return 0;
    VTMetalView *view = (__bridge VTMetalView *)handle;
    CVPixelBufferRef pixel = (CVPixelBufferRef)frame;
    if (!view.shown || !view.queue || !view.ciContext) return 0;
    CAMetalLayer *metal = view.metalLayer;
    id<CAMetalDrawable> drawable = [metal nextDrawable];
    if (!drawable) return 0;
    CGSize size = CGSizeMake(drawable.texture.width, drawable.texture.height);
    if (size.width < 1 || size.height < 1) return 0;
    size_t video_width = CVPixelBufferGetWidth(pixel);
    size_t video_height = CVPixelBufferGetHeight(pixel);
    if (video_width == 0 || video_height == 0) return 0;
    CGFloat sx = size.width / video_width, sy = size.height / video_height;
    if (view.keepAspect) {
        CGFloat scale = view.fillVideo ? MAX(sx, sy) : MIN(sx, sy);
        sx = sy = scale;
    }
    CGFloat draw_width = video_width * sx;
    CGFloat draw_height = video_height * sy;
    CGFloat dx = (size.width - draw_width) / 2;
    CGFloat dy = (size.height - draw_height) / 2;
    @try {
        CIImage *image = [CIImage imageWithCVPixelBuffer:pixel];
        @synchronized(view) { view.captureImage = image; }
        image = [image imageByApplyingTransform:CGAffineTransformMake(sx, 0, 0, sy, dx, dy)];
        CIImage *black = [[CIImage imageWithColor:[CIColor colorWithRed:0 green:0 blue:0 alpha:1]]
            imageByCroppingToRect:CGRectMake(0, 0, size.width, size.height)];
        CIImage *output = [image imageByCompositingOverImage:black];
        id<MTLCommandBuffer> buffer = [view.queue commandBuffer];
        if (!buffer) return 0;
        CFRetain(pixel);
        [buffer addCompletedHandler:^(id<MTLCommandBuffer> _Nonnull completed) {
            CFRelease(pixel);
        }];
        [view.ciContext render:output
                  toMTLTexture:drawable.texture
                 commandBuffer:buffer
                        bounds:CGRectMake(0, 0, size.width, size.height)
                    colorSpace:view.colorSpace];
        [buffer presentDrawable:drawable];
        [buffer commit];
        view.rendered++;
        return 1;
    } @catch (NSException *exception) {
        @synchronized(view) {
            view.lastError = exception.reason ?: @"CoreImage 渲染失败";
        }
        return 0;
    }
    }
}

uint64_t vt_view_rendered(void *handle) {
    if (!handle) return 0;
    VTMetalView *view = (__bridge VTMetalView *)handle;
    return view.rendered;
}

int vt_view_save_png(void *handle, const char *path, char *err, int errlen) {
    @autoreleasepool {
        if (!handle || !path || !*path) {
            if (err) snprintf(err, errlen, "画面尚未准备好");
            return -1;
        }
        VTMetalView *view = (__bridge VTMetalView *)handle;
        CIImage *image;
        @synchronized(view) { image = view.captureImage; }
        if (!image) { if (err) snprintf(err, errlen, "画面尚未准备好"); return -1; }
        CIContext *context = [CIContext contextWithOptions:@{kCIContextUseSoftwareRenderer: @YES}];
        CGImageRef bitmap = [context createCGImage:image fromRect:image.extent];
        if (!bitmap) { if (err) snprintf(err, errlen, "无法生成截图"); return -1; }
        NSURL *url = [NSURL fileURLWithPath:[NSString stringWithUTF8String:path]];
        CGImageDestinationRef output = CGImageDestinationCreateWithURL((__bridge CFURLRef)url, CFSTR("public.png"), 1, NULL);
        BOOL saved = NO;
        if (output) {
            CGImageDestinationAddImage(output, bitmap, NULL);
            saved = CGImageDestinationFinalize(output);
            CFRelease(output);
        }
        CGImageRelease(bitmap);
        if (!saved && err) snprintf(err, errlen, "截图写入失败，请检查保存位置");
        return saved ? 0 : -1;
    }
}

const char *vt_view_error(void *handle) {
    if (!handle) return NULL;
    VTMetalView *view = (__bridge VTMetalView *)handle;
    @synchronized(view) {
        return view.lastError ? view.lastError.UTF8String : NULL;
    }
}
