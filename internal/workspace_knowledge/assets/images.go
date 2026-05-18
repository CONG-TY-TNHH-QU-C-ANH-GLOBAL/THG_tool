package assets

import (
	"encoding/json"
	"strings"
)

// Image-aware outreach (Phase E).
//
// PRODUCT DECISION: per CLAUDE.md ("Do not generate AI images. Use
// real uploaded files/images only"), the system never produces image
// content. It only ATTACHES images that the operator (or the source
// system) already uploaded. So Phase E is about plumbing real image
// URLs from the Asset payload into the outbound runtime.
//
// PAYLOAD SHAPE: an Asset's payload (JSON object) MAY contain:
//
//   {
//     ...                              // type-specific fields
//     "images": [                      // ordered, primary first
//       {
//         "url": "https://cdn.thgfulfill.com/...",
//         "alt": "Custom Cat Tee front",
//         "kind": "primary"            // "primary" | "alternate" | "mockup"
//       },
//       ...
//     ]
//   }
//
// The runtime browser layer is the consumer — it downloads the URL,
// uploads to Facebook's compose surface, and includes the alt text
// as the photo description.
//
// IMAGE STORAGE: not handled here. Ingestors store CDN-hosted URLs
// that the runtime can fetch; the system does not download or proxy
// images itself. (Shopify hosts product images; CSV uploads must
// reference a public URL or a pre-uploaded asset URL.)
//
// FAILURE MODE: a corrupted images payload returns no images and
// logs nothing. We do NOT fail asset retrieval just because the
// image list parsed badly — text-only outreach is the safe fallback.

// AssetImage is one image attached to an asset.
type AssetImage struct {
	URL  string
	Alt  string
	Kind string // "primary" | "alternate" | "mockup" | "" (treated as primary)
}

// ImagesFromPayload extracts the image list embedded in an asset's
// payload. Returns nil if the payload has no images, is not a JSON
// object, or is malformed. The caller decides whether to attach
// images at all (some lead contexts are text-only by policy).
//
// The function is intentionally permissive: missing "alt", a payload
// that is an array (instead of an object), or an "images" field that
// is a string return empty cleanly. Only well-formed entries with a
// non-empty URL flow through.
func ImagesFromPayload(payload json.RawMessage) []AssetImage {
	if len(payload) == 0 {
		return nil
	}
	// Two valid shapes: object with "images", or object with no
	// "images" key. Anything else (array, string, number) is treated
	// as "no images" rather than an error.
	var obj struct {
		Images json.RawMessage `json:"images"`
	}
	if err := json.Unmarshal(payload, &obj); err != nil || len(obj.Images) == 0 {
		return nil
	}
	var raw []struct {
		URL  string `json:"url"`
		Alt  string `json:"alt"`
		Kind string `json:"kind"`
	}
	if err := json.Unmarshal(obj.Images, &raw); err != nil {
		return nil
	}
	out := make([]AssetImage, 0, len(raw))
	for _, im := range raw {
		url := strings.TrimSpace(im.URL)
		if url == "" {
			continue
		}
		out = append(out, AssetImage{
			URL:  url,
			Alt:  strings.TrimSpace(im.Alt),
			Kind: strings.ToLower(strings.TrimSpace(im.Kind)),
		})
	}
	return out
}

// PrimaryImage returns the first image marked "primary", or the
// first entry if none is marked. nil if the asset has no images.
// Most outbound flows want exactly one image; this is the helper for
// that hot path.
func PrimaryImage(payload json.RawMessage) *AssetImage {
	imgs := ImagesFromPayload(payload)
	if len(imgs) == 0 {
		return nil
	}
	for i := range imgs {
		if imgs[i].Kind == "primary" || imgs[i].Kind == "" {
			return &imgs[i]
		}
	}
	return &imgs[0]
}
