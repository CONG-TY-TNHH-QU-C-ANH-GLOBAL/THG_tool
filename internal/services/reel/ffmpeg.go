package reel

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// muxAudio overlays a TTS mp3 onto a silent video clip (used for broll/product shots whose
// FAL clip has no audio). Video is copied; audio is re-encoded to aac; -shortest trims to the
// shorter stream so a long voiceover doesn't pad the clip with a frozen frame.
func muxAudio(ctx context.Context, ffmpegPath, video, audio, out string) error {
	cmd := exec.CommandContext(ctx, ffmpegPath, "-y",
		"-i", video, "-i", audio,
		"-map", "0:v:0", "-map", "1:a:0",
		"-c:v", "copy", "-c:a", "aac", "-shortest", out)
	return runFFmpeg(cmd)
}

// concatVideos normalises every shot clip to a common 720x1280@30fps + 44.1kHz stereo format
// (clips come from different providers with different sizes/SAR/audio rates) and concatenates
// them in order into one reel. When logoPath is set, the brand icon is overlaid on the whole
// reel at the top-left corner. A single filter_complex avoids intermediate files.
func concatVideos(ctx context.Context, ffmpegPath string, clips []string, logoPath, out string) error {
	if len(clips) == 0 {
		return fmt.Errorf("ffmpeg concat: no clips")
	}
	args := []string{"-y"}
	for _, c := range clips {
		args = append(args, "-i", c)
	}
	hasLogo := strings.TrimSpace(logoPath) != ""
	if hasLogo {
		args = append(args, "-i", logoPath) // logo is the last input (index == len(clips))
	}
	var fc strings.Builder
	for i := range clips {
		idx := strconv.Itoa(i)
		// Scale to fit, pad to exact frame, normalise SAR + fps for video; resample audio.
		fc.WriteString("[" + idx + ":v]scale=720:1280:force_original_aspect_ratio=decrease," +
			"pad=720:1280:(ow-iw)/2:(oh-ih)/2,setsar=1,fps=30[v" + idx + "];")
		fc.WriteString("[" + idx + ":a]aresample=44100,aformat=sample_fmts=fltp:channel_layouts=stereo[a" + idx + "];")
	}
	for i := range clips {
		idx := strconv.Itoa(i)
		fc.WriteString("[v" + idx + "][a" + idx + "]")
	}
	// Concat to a label we may post-process with the watermark overlay.
	vlabel := "[outv]"
	if hasLogo {
		vlabel = "[cv]"
	}
	fc.WriteString("concat=n=" + strconv.Itoa(len(clips)) + ":v=1:a=1" + vlabel + "[outa]")
	if hasLogo {
		// Scale the logo to 120px wide (keep aspect) and pin it to the top-left corner with a
		// 28px margin over the whole reel timeline.
		logoIdx := strconv.Itoa(len(clips))
		fc.WriteString(";[" + logoIdx + ":v]scale=120:-1[lg];[cv][lg]overlay=28:28[outv]")
	}

	args = append(args,
		"-filter_complex", fc.String(),
		"-map", "[outv]", "-map", "[outa]",
		"-c:v", "libx264", "-c:a", "aac", "-pix_fmt", "yuv420p", out)
	return runFFmpeg(exec.CommandContext(ctx, ffmpegPath, args...))
}

// runFFmpeg executes the command, surfacing the tail of stderr on failure so a bad filter or
// missing binary is diagnosable from the log (degrade honestly, never panic).
func runFFmpeg(cmd *exec.Cmd) error {
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ffmpeg: %w: %s", err, tailStr(stderr.String(), 500))
	}
	return nil
}

// tailStr returns the last n runes of s (ffmpeg writes the actual error at the end of stderr,
// after the version banner / stream-mapping noise).
func tailStr(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return "..." + string(r[len(r)-n:])
}
