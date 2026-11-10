#define GL_SILENCE_DEPRECATION
#import <AppKit/AppKit.h>
#import <AVKit/AVKit.h>
#import <CoreVideo/CoreVideo.h>
#import <OpenGL/gl3.h>
#include "picture_in_picture_darwin.h"

extern void liveMyGoPictureInPictureEvent(uintptr_t handle, int event);

enum { LMPiPStopped = 1, LMPiPFailed, LMPiPPlay, LMPiPPause };

@interface LMPictureInPicture : NSObject <AVPictureInPictureControllerDelegate, AVPictureInPictureSampleBufferPlaybackDelegate>
@property(nonatomic) uintptr_t callback;
@property(nonatomic) BOOL paused;
@property(nonatomic, weak) NSWindow *window;
@property(nonatomic, strong) NSView *host;
@property(nonatomic, strong) AVSampleBufferDisplayLayer *layer;
@property(nonatomic, strong) AVPictureInPictureController *controller;
- (void)startWhenReady:(int)attempt;
@end

@implementation LMPictureInPicture

- (void)emit:(int)event {
    dispatch_async(dispatch_get_main_queue(), ^{
        if (self.callback != 0) liveMyGoPictureInPictureEvent(self.callback, event);
    });
}

- (void)startWhenReady:(int)attempt {
    if (self.callback == 0) return;
    if (self.controller.pictureInPicturePossible) {
        [self.controller startPictureInPicture];
    } else if (attempt < 25) {
        dispatch_after(dispatch_time(DISPATCH_TIME_NOW, 80 * NSEC_PER_MSEC), dispatch_get_main_queue(), ^{
            [self startWhenReady:attempt + 1];
        });
    } else {
        [self emit:LMPiPFailed];
    }
}

- (void)pictureInPictureControllerDidStopPictureInPicture:(AVPictureInPictureController *)controller {
    [self emit:LMPiPStopped];
}

- (void)pictureInPictureController:(AVPictureInPictureController *)controller failedToStartPictureInPictureWithError:(NSError *)error {
    [self emit:LMPiPFailed];
}

- (void)pictureInPictureController:(AVPictureInPictureController *)controller restoreUserInterfaceForPictureInPictureStopWithCompletionHandler:(void (^)(BOOL))completion {
    NSWindow *window = self.window;
    if (window.miniaturized) [window deminiaturize:nil];
    [window makeKeyAndOrderFront:nil];
    completion(window != nil);
}

- (void)pictureInPictureController:(AVPictureInPictureController *)controller setPlaying:(BOOL)playing {
    [self emit:playing ? LMPiPPlay : LMPiPPause];
}

- (CMTimeRange)pictureInPictureControllerTimeRangeForPlayback:(AVPictureInPictureController *)controller {
    return CMTimeRangeMake(kCMTimeZero, kCMTimePositiveInfinity);
}

- (BOOL)pictureInPictureControllerIsPlaybackPaused:(AVPictureInPictureController *)controller { return self.paused; }

- (void)pictureInPictureController:(AVPictureInPictureController *)controller didTransitionToRenderSize:(CMVideoDimensions)size {}

- (void)pictureInPictureController:(AVPictureInPictureController *)controller skipByInterval:(CMTime)interval completionHandler:(void (^)(void))completion {
    completion();
}

@end

void *lm_pip_create(uintptr_t window, uintptr_t video_view, uintptr_t callback, int paused) {
    NSWindow *nativeWindow = (__bridge NSWindow *)(void *)window;
    NSView *videoView = (__bridge NSView *)(void *)video_view;
    if (![AVPictureInPictureController isPictureInPictureSupported] || nativeWindow.contentView == nil || videoView == nil) return NULL;
    LMPictureInPicture *picture = [LMPictureInPicture new];
    picture.callback = callback;
    picture.paused = paused != 0;
    picture.window = nativeWindow;
    picture.layer = [AVSampleBufferDisplayLayer new];
    picture.layer.videoGravity = AVLayerVideoGravityResizeAspect;
    picture.host = [[NSView alloc] initWithFrame:videoView.frame];
    picture.host.wantsLayer = YES;
    picture.host.layer = picture.layer;
    [nativeWindow.contentView addSubview:picture.host positioned:NSWindowBelow relativeTo:videoView];
    AVPictureInPictureControllerContentSource *source = [[AVPictureInPictureControllerContentSource alloc] initWithSampleBufferDisplayLayer:picture.layer playbackDelegate:picture];
    picture.controller = [[AVPictureInPictureController alloc] initWithContentSource:source];
    if (picture.controller == nil) {
        [picture.host removeFromSuperview];
        return NULL;
    }
    picture.controller.delegate = picture;
    [picture startWhenReady:0];
    return (__bridge_retained void *)picture;
}

void lm_pip_close(void *handle) {
    LMPictureInPicture *picture = (__bridge_transfer LMPictureInPicture *)handle;
    picture.callback = 0;
    picture.controller.delegate = nil;
    if (picture.controller.pictureInPictureActive) [picture.controller stopPictureInPicture];
    [picture.host removeFromSuperview];
}

void lm_pip_pause(void *handle, int paused) {
    LMPictureInPicture *picture = (__bridge LMPictureInPicture *)handle;
    if (picture.paused != (paused != 0)) {
        picture.paused = paused != 0;
        [picture.controller invalidatePlaybackState];
    }
}

int lm_pip_enqueue(void *handle, void *pixel_buffer) {
    @autoreleasepool {
        LMPictureInPicture *picture = (__bridge LMPictureInPicture *)handle;
        AVSampleBufferVideoRenderer *renderer = picture.layer.sampleBufferRenderer;
        if (!renderer.readyForMoreMediaData) return 0;
        CVPixelBufferRef buffer = pixel_buffer;
        CMVideoFormatDescriptionRef format = NULL;
        if (CMVideoFormatDescriptionCreateForImageBuffer(kCFAllocatorDefault, buffer, &format) != noErr) return 0;
        CMSampleTimingInfo timing = {kCMTimeInvalid, CMClockGetTime(CMClockGetHostTimeClock()), kCMTimeInvalid};
        CMSampleBufferRef sample = NULL;
        OSStatus rc = CMSampleBufferCreateReadyWithImageBuffer(kCFAllocatorDefault, buffer, format, &timing, &sample);
        CFRelease(format);
        if (rc != noErr) return 0;
        CFArrayRef attachments = CMSampleBufferGetSampleAttachmentsArray(sample, true);
        if (attachments != NULL && CFArrayGetCount(attachments) > 0) {
            CFDictionarySetValue((CFMutableDictionaryRef)CFArrayGetValueAtIndex(attachments, 0), kCMSampleAttachmentKey_DisplayImmediately, kCFBooleanTrue);
        }
        [renderer enqueueSampleBuffer:sample];
        CFRelease(sample);
        return 1;
    }
}

typedef struct {
    int width, height;
    GLuint texture, fbo;
    CVPixelBufferPoolRef pool;
} LMGLPictureTarget;

void *lm_pip_gl_create(int width, int height, uint32_t *fbo) {
    @autoreleasepool {
        LMGLPictureTarget *target = calloc(1, sizeof(*target));
        if (target == NULL) return NULL;
        target->width = width;
        target->height = height;
        NSDictionary *attributes = @{(id)kCVPixelBufferWidthKey: @(width), (id)kCVPixelBufferHeightKey: @(height),
                                     (id)kCVPixelBufferPixelFormatTypeKey: @(kCVPixelFormatType_32BGRA),
                                     (id)kCVPixelBufferIOSurfacePropertiesKey: @{}};
        if (CVPixelBufferPoolCreate(kCFAllocatorDefault, NULL, (__bridge CFDictionaryRef)attributes, &target->pool) != kCVReturnSuccess) {
            lm_pip_gl_close(target);
            return NULL;
        }
        glGenTextures(1, &target->texture);
        glBindTexture(GL_TEXTURE_2D, target->texture);
        glTexImage2D(GL_TEXTURE_2D, 0, GL_RGBA8, width, height, 0, GL_RGBA, GL_UNSIGNED_BYTE, NULL);
        glGenFramebuffers(1, &target->fbo);
        glBindFramebuffer(GL_FRAMEBUFFER, target->fbo);
        glFramebufferTexture2D(GL_FRAMEBUFFER, GL_COLOR_ATTACHMENT0, GL_TEXTURE_2D, target->texture, 0);
        GLenum status = glCheckFramebufferStatus(GL_FRAMEBUFFER);
        glBindFramebuffer(GL_FRAMEBUFFER, 0);
        glBindTexture(GL_TEXTURE_2D, 0);
        if (status != GL_FRAMEBUFFER_COMPLETE) {
            lm_pip_gl_close(target);
            return NULL;
        }
        *fbo = target->fbo;
        return target;
    }
}

void lm_pip_gl_close(void *handle) {
    LMGLPictureTarget *target = handle;
    if (target == NULL) return;
    glDeleteFramebuffers(1, &target->fbo);
    glDeleteTextures(1, &target->texture);
    if (target->pool != NULL) CFRelease(target->pool);
    free(target);
}

int lm_pip_gl_enqueue(void *picture, void *handle) {
    @autoreleasepool {
        LMGLPictureTarget *target = handle;
        NSDictionary *limits = @{(id)kCVPixelBufferPoolAllocationThresholdKey: @4};
        CVPixelBufferRef buffer = NULL;
        if (CVPixelBufferPoolCreatePixelBufferWithAuxAttributes(kCFAllocatorDefault, target->pool, (__bridge CFDictionaryRef)limits, &buffer) != kCVReturnSuccess) return 0;
        if (CVPixelBufferLockBaseAddress(buffer, 0) != kCVReturnSuccess) {
            CVPixelBufferRelease(buffer);
            return 0;
        }
        glBindFramebuffer(GL_FRAMEBUFFER, target->fbo);
        glPixelStorei(GL_PACK_ROW_LENGTH, (GLint)CVPixelBufferGetBytesPerRow(buffer) / 4);
        glReadPixels(0, 0, target->width, target->height, GL_BGRA, GL_UNSIGNED_BYTE, CVPixelBufferGetBaseAddress(buffer));
        glPixelStorei(GL_PACK_ROW_LENGTH, 0);
        glBindFramebuffer(GL_FRAMEBUFFER, 0);
        CVPixelBufferUnlockBaseAddress(buffer, 0);
        int ok = lm_pip_enqueue(picture, buffer);
        CVPixelBufferRelease(buffer);
        return ok;
    }
}
