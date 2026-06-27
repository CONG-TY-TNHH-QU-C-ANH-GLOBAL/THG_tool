package reel

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	reelstore "github.com/thg/scraper/internal/store/reel"
)

// assembleFinal stitches the done shot clips into one final.mp4 once every shot is done.
//
// When ffmpeg/media is NOT configured (FakeRenderer path, tests, CI) it preserves the prior
// synthetic behavior — set a placeholder final key and advance to assembled — so the
// zero-cost flow still reaches a publishable state. With real media configured it concatenates
// the actual shot clips locally; an ffmpeg failure leaves the reel at render_done (logged, not
// crashed) so it simply isn't publishable yet rather than taking the server down.
func (s *Service) assembleFinal(orgID, reelID int64) error {
	if s.render.FFmpegPath == "" || s.render.MediaDir == "" {
		finalKey := fmt.Sprintf("renders/reel-%d/final.mp4", reelID)
		return s.db.Reel().SetFinalOutput(orgID, reelID, finalKey, reelstore.StatusAssembled)
	}

	shots, err := s.db.Reel().ListShots(orgID, reelID)
	if err != nil {
		return err
	}
	clips := make([]string, 0, len(shots))
	for _, sh := range shots {
		if sh.RenderState != reelstore.ShotDone || sh.OutputKey == "" {
			continue
		}
		clips = append(clips, filepath.Join(s.render.MediaDir, filepath.FromSlash(sh.OutputKey)))
	}
	if len(clips) == 0 {
		log.Printf("[reel] assemble reel=%d: no done clips to concat", reelID)
		return nil
	}

	relKey := fmt.Sprintf("reel-%d/final.mp4", reelID)
	finalPath := filepath.Join(s.render.MediaDir, filepath.FromSlash(relKey))
	ctx, cancel := context.WithTimeout(context.Background(), assembleTTL)
	defer cancel()
	if err := concatVideos(ctx, s.render.FFmpegPath, clips, s.render.LogoPath, finalPath); err != nil {
		// Degrade honestly: stay at render_done so a human/retry can reassemble.
		log.Printf("[reel] assemble reel=%d ffmpeg concat failed: %v", reelID, err)
		return nil
	}
	return s.db.Reel().SetFinalOutput(orgID, reelID, relKey, reelstore.StatusAssembled)
}

// VideoPath resolves a reel's final video to an absolute on-disk path for the serve endpoint.
// It validates the reel has a final output, the media dir is configured, and (path-traversal
// guard) the resolved file stays under MediaDir, then confirms the file exists.
func (s *Service) VideoPath(orgID, reelID int64) (string, error) {
	r, err := s.db.Reel().GetReel(orgID, reelID)
	if err != nil {
		return "", err
	}
	if r.FinalOutputKey == "" {
		return "", fmt.Errorf("reel: no rendered video")
	}
	if s.render.MediaDir == "" {
		return "", fmt.Errorf("reel: media storage not configured")
	}
	root, err := filepath.Abs(s.render.MediaDir)
	if err != nil {
		return "", err
	}
	abs, err := filepath.Abs(filepath.Join(root, filepath.FromSlash(r.FinalOutputKey)))
	if err != nil {
		return "", err
	}
	if abs != root && !strings.HasPrefix(abs, root+string(os.PathSeparator)) {
		return "", fmt.Errorf("reel: invalid video path")
	}
	if _, err := os.Stat(abs); err != nil {
		return "", fmt.Errorf("reel: video file unavailable: %w", err)
	}
	return abs, nil
}
