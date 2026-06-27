package reels

import "github.com/gofiber/fiber/v2"

// serveVideo streams a reel's assembled final.mp4 (JWT, org-scoped). The service validates
// the file exists under the configured media dir (path-traversal guarded); a missing/unready
// video is a 404 so the client can distinguish "not done" from a server fault.
func serveVideo(deps Deps) fiber.Handler {
	return func(c *fiber.Ctx) error {
		orgID, _ := orgAndUser(c)
		id, ok := reelID(c)
		if !ok {
			return nil
		}
		path, err := deps.Service.VideoPath(orgID, id)
		if err != nil {
			// Reel missing (tenant/404) or video not yet assembled both mean "no playable
			// file here" — surface 404 so the client polls rather than treating it as a fault.
			return c.Status(404).JSON(fiber.Map{"error": err.Error()})
		}
		c.Set("Content-Type", "video/mp4")
		return c.SendFile(path, true)
	}
}
