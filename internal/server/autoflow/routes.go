package autoflow

import (
	"github.com/gofiber/fiber/v2"
	"github.com/thg/scraper/internal/store"
)

// Deps holds dependencies needed by AutoFlow handlers.
type Deps struct {
	DB *store.Store
}

type Handler struct {
	deps Deps
}

// Routes registers AutoFlow workspace endpoints.
func Routes(group fiber.Router, deps Deps, adminOnly fiber.Handler) {
	h := &Handler{deps: deps}

	group.Get("/staff", h.autoflowGetStaff)
	group.Put("/staff/:id/kpi", adminOnly, h.autoflowUpdateKPI)

	group.Get("/kpi/config", h.autoflowGetKPIConfig)
	group.Put("/kpi/config", adminOnly, h.autoflowUpdateKPIConfig)

	group.Get("/files", h.autoflowListFiles)
	group.Post("/files", h.autoflowUploadFile)
	group.Delete("/files/:id", h.autoflowDeleteFile)

	group.Get("/data-sources", h.listDataSources)
	group.Post("/data-sources", adminOnly, h.createDataSource)
	group.Post("/data-sources/:id/sync", adminOnly, h.syncDataSource)
	group.Delete("/data-sources/:id", adminOnly, h.deleteDataSource)

	group.Get("/threads", h.autoflowListThreads)
	group.Get("/threads/:id/messages", h.autoflowGetMessages)
	group.Post("/threads/:id/messages", h.autoflowSendMessage)

	group.Get("/facebook/status", h.autoflowFacebookStatus)
	group.Get("/context/business", h.getBusinessContext)
	group.Put("/context/business", h.updateBusinessContext)
	group.Get("/billing/summary", h.billingSummary)
}
