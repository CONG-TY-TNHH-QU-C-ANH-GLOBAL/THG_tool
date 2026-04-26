package parser

import (
	"context"

	"github.com/thg/scraper/internal/jobs"
)

// Parser converts natural language text into a structured Task.
type Parser interface {
	Parse(ctx context.Context, text string) (*jobs.Task, error)
}
