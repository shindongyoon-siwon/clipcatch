package main

import "strings"

// Safe fixed labels, not source URLs or raw logs. A candidate limitation must
// never be presented as proof that a video owner prohibits partial downloads.
func clipMetadataFeedback(live bool) (string, string) {
	if live {
		return "live", "이 시험판은 실시간 영상의 구간 저장을 지원하지 않아요."
	}
	return "duration", "영상 길이를 확인하지 못해 정확한 구간 저장을 시작할 수 없어요."
}
func clipStaticFormatReason(f clipFormat) string {
	if (f.HDR != "" && f.HDR != "SDR") || f.FPS > 120 {
		return "hdr_fps"
	}
	if !clipMediaURL(f.URL) {
		return "source_address"
	}
	if f.Protocol == "m3u8" || f.Protocol == "m3u8_native" || strings.Contains(strings.Split(f.URL, "?")[0], ".m3u8") {
		return ""
	}
	if f.Protocol != "" && f.Protocol != "http" && f.Protocol != "https" {
		return "stream_protocol"
	}
	if f.Ext == "webm" {
		return "webm"
	}
	if f.Ext != "mp4" && f.Ext != "m4a" {
		return "container"
	}
	return ""
}
func clipChoiceFeedback(c clipChoice) (string, string) {
	if c.Status == "supported" {
		return "ready", c.Notice
	}
	if c.Status == "unknown" {
		return "unconfirmed", clipNotice("unknown")
	}
	if c.Selector == "" {
		return "format_selection", "선택한 화질의 영상·음성 조합을 확인하지 못했어요."
	}
	kind, reason := "video", clipStaticFormatReason(c.Video.Format)
	if reason == "" && c.Audio != nil {
		kind, reason = "audio", clipStaticFormatReason(c.Audio.Format)
	}
	if reason == "webm" {
		if kind == "audio" {
			return "audio_webm", "현재 시험판은 선택된 WebM 음성 형식의 구간 저장을 처리하지 못해요. 영상 자체의 구간 저장 금지를 뜻하지는 않아요."
		}
		return "video_webm", "현재 시험판은 선택된 WebM 영상 형식의 구간 저장을 처리하지 못해요. 다른 화질을 선택하면 다시 확인해요."
	}
	switch reason {
	case "stream_protocol":
		return kind + "_protocol", "현재 시험판이 처리하지 못하는 스트리밍 형식이에요. 영상 자체의 구간 저장 금지로 확정한 것은 아니에요."
	case "container":
		return kind + "_container", "현재 시험판은 선택된 파일 형식의 구간 저장을 지원하지 않아요."
	case "hdr_fps":
		return "hdr_fps", "이 시험판은 HDR 또는 120fps 초과 영상의 구간 저장을 지원하지 않아요."
	case "source_address":
		return "source_address", "구간 다운로드에 사용할 수 있는 영상 주소를 확인하지 못했어요."
	}
	if c.Video.List == nil && (c.Video.Format.Protocol == "m3u8" || c.Video.Format.Protocol == "m3u8_native") {
		return "hls_layout", "선택된 스트리밍 구조를 현재 시험판에서 안전하게 나누어 받을 수 없어요."
	}
	return "partial_response", "선택한 화질에서 전체 영상 없이 부분만 받을 수 있는지 확인하지 못해 중단했어요. 전체로 자동 전환하지 않아요."
}
